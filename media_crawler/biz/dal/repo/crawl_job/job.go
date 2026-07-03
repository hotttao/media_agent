package crawl_job

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"media_agent/media_crawler/biz/dal"
	"media_agent/media_crawler/biz/dal/model"
	"media_agent/media_crawler/biz/dal/query"
	domain "media_agent/media_crawler/biz/domain/crawl_job"
)

// JobRepo 实现 domain.JobRepo（PG，经 GORM Gen query）。
// 从事务 ctx 取连接（dal.FromContext），支持上层 RunInTx 透传。
type JobRepo struct{ db *gorm.DB }

func NewJobRepo(db *gorm.DB) *JobRepo { return &JobRepo{db: db} }

func (r *JobRepo) Save(ctx context.Context, job *domain.Job) error {
	return query.Use(dal.FromContext(ctx, r.db)).Job.WithContext(ctx).Create(toModel(job))
}

func (r *JobRepo) Get(ctx context.Context, jobID string) (*domain.Job, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	m, err := Q.Job.WithContext(ctx).Where(Q.Job.JobID.Eq(jobID)).First()
	if err != nil {
		return nil, err
	}
	return toDomain(m), nil
}

func (r *JobRepo) UpdateRunning(ctx context.Context, jobID string, pid int) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Job.WithContext(ctx).Where(Q.Job.JobID.Eq(jobID)).Updates(map[string]any{
		"status":     string(domain.StatusRunning),
		"pid":        pid,
		"started_at": time.Now(),
	})
	return err
}

func (r *JobRepo) UpdateProgress(ctx context.Context, jobID string, progress map[string]any) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Job.WithContext(ctx).Where(Q.Job.JobID.Eq(jobID)).Updates(map[string]any{"progress": toJson(progress)})
	return err
}

func (r *JobRepo) UpdateResult(ctx context.Context, jobID string, status domain.Status, summary map[string]any, errMsg string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Job.WithContext(ctx).Where(Q.Job.JobID.Eq(jobID)).Updates(map[string]any{
		"status":         string(status),
		"result_summary": toJson(summary),
		"error":          errMsg,
		"finished_at":    time.Now(),
		"pid":            nil,
	})
	return err
}

func (r *JobRepo) UpdateCheckpoint(ctx context.Context, jobID, checkpointID string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Job.WithContext(ctx).Where(Q.Job.JobID.Eq(jobID)).Updates(map[string]any{"checkpoint_id": checkpointID})
	return err
}

func (r *JobRepo) UpdateStdoutTail(ctx context.Context, jobID, tail string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Job.WithContext(ctx).Where(Q.Job.JobID.Eq(jobID)).Updates(map[string]any{"stdout_tail": tail})
	return err
}

func (r *JobRepo) List(ctx context.Context, platform, status string, limit, offset int) ([]*domain.Job, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	do := Q.Job.WithContext(ctx)
	if platform != "" {
		do = do.Where(Q.Job.Platform.Eq(platform))
	}
	if status != "" {
		do = do.Where(Q.Job.Status.Eq(status))
	}
	total, err := do.Count()
	if err != nil {
		return nil, 0, err
	}
	rows, err := do.Order(Q.Job.QueuedAt.Desc()).Limit(limit).Offset(offset).Find()
	if err != nil {
		return nil, 0, err
	}
	jobs := make([]*domain.Job, len(rows))
	for i, m := range rows {
		jobs[i] = toDomain(m)
	}
	return jobs, int(total), nil
}

func toModel(j *domain.Job) *model.Job {
	return &model.Job{
		JobID:         j.JobID,
		Platform:      j.Platform,
		CrawlerType:   j.CrawlerType,
		Params:        toJson(j.Params),
		Status:        string(j.Status),
		Pid:           j.Pid,
		CheckpointID:  j.CheckpointID,
		Progress:      toJson(j.Progress),
		ResultSummary: toJson(j.ResultSummary),
		Error:         j.Error,
		StdoutTail:    j.StdoutTail,
		QueuedAt:      j.QueuedAt,
		StartedAt:     j.StartedAt,
		FinishedAt:    j.FinishedAt,
	}
}

func toDomain(m *model.Job) *domain.Job {
	return &domain.Job{
		JobID:         m.JobID,
		Platform:      m.Platform,
		CrawlerType:   m.CrawlerType,
		Params:        fromJson(m.Params),
		Status:        domain.Status(m.Status),
		Pid:           m.Pid,
		CheckpointID:  m.CheckpointID,
		Progress:      fromJson(m.Progress),
		ResultSummary: fromJson(m.ResultSummary),
		Error:         m.Error,
		StdoutTail:    m.StdoutTail,
		QueuedAt:      m.QueuedAt,
		StartedAt:     m.StartedAt,
		FinishedAt:    m.FinishedAt,
	}
}

func toJson(m map[string]any) datatypes.JSON {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return datatypes.JSON(b)
}

func fromJson(j datatypes.JSON) map[string]any {
	if len(j) == 0 {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(j, &m)
	return m
}

var _ domain.JobRepo = (*JobRepo)(nil)
