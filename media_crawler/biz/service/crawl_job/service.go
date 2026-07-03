package crawl_job

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	configpb "media_agent/hertz_gen/config"

	"media_agent/media_crawler/biz/dal"
	domain "media_agent/media_crawler/biz/domain/crawl_job"
)

// Service crawl_job 用例编排。持有 JobRepo + TxRunner + CrawlerConfig + runner（subprocess）。
// start_crawler 含外部 subprocess 副作用，事务遵守"写 job（事务内）→ commit → subprocess（事务外）"。
type Service struct {
	repo   domain.JobRepo
	tx     dal.TxRunner
	cfg    *configpb.CrawlerConfig
	runner *runner
}

func NewService(repo domain.JobRepo, tx dal.TxRunner, cfg *configpb.CrawlerConfig) *Service {
	s := &Service{repo: repo, tx: tx, cfg: cfg}
	s.runner = newRunner(repo, cfg)
	return s
}

// StartCrawler 提交抓取任务：写 job（PG 事务）→ commit → 异步 subprocess。
// 立即返回 job_id（wait_for_completion=false 默认）。
func (s *Service) StartCrawler(ctx context.Context, cmd StartCrawlerCommand) (string, error) {
	if err := validate(cmd); err != nil {
		return "", err
	}
	jobID := genJobID()
	job := &domain.Job{
		JobID:       jobID,
		Platform:    cmd.Platform,
		CrawlerType: cmd.CrawlerType,
		Params:      toParams(cmd),
		Status:      domain.StatusQueued,
		QueuedAt:    time.Now(),
	}
	if err := s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		return s.repo.Save(txCtx, job)
	}); err != nil {
		return "", err
	}
	// subprocess 在事务外异步执行（外部副作用，不持 DB 事务）
	go s.runner.start(jobID, cmd)
	return jobID, nil
}

// GetCrawlStatus 查 job 状态（含 progress/result_summary）。
func (s *Service) GetCrawlStatus(ctx context.Context, jobID string) (*domain.Job, error) {
	return s.repo.Get(ctx, jobID)
}

// StopCrawler kill 子进程（幂等：已终态直接返回）。
func (s *Service) StopCrawler(ctx context.Context, jobID string, signal string) error {
	job, err := s.repo.Get(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status.IsTerminal() {
		return nil // 幂等
	}
	if err := s.runner.stop(jobID, signal); err != nil {
		return err
	}
	return s.repo.UpdateResult(ctx, jobID, domain.StatusCancelled, nil, "stopped by user")
}

// ListJobs 分页列 job。
func (s *Service) ListJobs(ctx context.Context, q ListJobsQuery) ([]*domain.Job, int, error) {
	return s.repo.List(ctx, q.Platform, q.Status, q.Limit, q.Offset)
}

// GetJob 任务详情（同 GetCrawlStatus）。
func (s *Service) GetJob(ctx context.Context, jobID string) (*domain.Job, error) {
	return s.repo.Get(ctx, jobID)
}

// ---- 校验 / 辅助 ----

func validate(cmd StartCrawlerCommand) error {
	validPlatforms := map[string]bool{
		"xhs": true, "dy": true, "ks": true, "bili": true, "wb": true, "tieba": true, "zhihu": true,
	}
	if !validPlatforms[cmd.Platform] {
		return fmt.Errorf("invalid platform %q", cmd.Platform)
	}
	switch cmd.CrawlerType {
	case "search":
		if cmd.Keywords == "" {
			return fmt.Errorf("search requires keywords")
		}
	case "detail", "creator":
		if len(cmd.URLs) == 0 {
			return fmt.Errorf("%s requires urls", cmd.CrawlerType)
		}
	case "homefeed", "hot_search":
		// 无必填
	default:
		return fmt.Errorf("invalid crawler_type %q", cmd.CrawlerType)
	}
	return nil
}

func genJobID() string {
	// job_{YYYYMMDDHHMMSS}_{6位随机}
	ts := time.Now().Format("20060102150405")
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	return "job_" + ts + "_" + id
}

func toParams(cmd StartCrawlerCommand) map[string]any {
	return map[string]any{
		"keywords":                  cmd.Keywords,
		"urls":                      cmd.URLs,
		"enable_comments":           cmd.EnableComments,
		"enable_sub_comments":       cmd.EnableSubComments,
		"max_notes_count":           cmd.MaxNotesCount,
		"save_data_option":          cmd.SaveDataOption,
		"start_page":                cmd.StartPage,
		"enable_checkpoint":         cmd.EnableCheckpoint,
		"checkpoint_id":             cmd.CheckpointID,
		"hot_search_max_items":      cmd.HotSearchMaxItems,
		"hot_search_per_item_count": cmd.HotSearchPerItemCount,
	}
}

// intSeq 辅助（占位，避免未用 import）。
var _ = strconv.Itoa
