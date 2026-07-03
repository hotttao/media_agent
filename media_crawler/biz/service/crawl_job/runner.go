package crawl_job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	configpb "media_agent/hertz_gen/config"

	domain "media_agent/media_crawler/biz/domain/crawl_job"
)

// runner 负责 subprocess 调用 Python 爬虫（mcp_runner.py 垫片）。
// 全局串行（mu 互斥）：同时仅一个爬虫子进程，防账号/代理风控。
// 不抽 shared（用户决定 start_crawler 归 service，subprocess 内联在此包）。
type runner struct {
	repo  domain.JobRepo
	cfg   *configpb.CrawlerConfig
	mu    sync.Mutex // 串行锁
	procs map[string]*exec.Cmd
}

func newRunner(repo domain.JobRepo, cfg *configpb.CrawlerConfig) *runner {
	return &runner{repo: repo, cfg: cfg, procs: map[string]*exec.Cmd{}}
}

var crawlSummaryRe = regexp.MustCompile(`---CRAWL_SUMMARY:\s*(\{.*?\})\s*---`)

// start 启动子进程并监控到结束。串行：持 mu 直到子进程退出。
func (r *runner) start(jobID string, cmd StartCrawlerCommand) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctx := context.Background()
	cliArgs := buildCLIArgs(cmd)
	c := exec.CommandContext(ctx, r.cfg.GetPythonPath(), r.cfg.GetRunnerPath())
	c.Dir = r.cfg.GetWorkDir()
	env := append(os.Environ(), "MCP_CLI_ARGS="+strings.Join(cliArgs, " "))
	if cmd.MaxNotesCount > 0 {
		env = append(env, fmt.Sprintf("MCP_MAX_NOTES_COUNT=%d", cmd.MaxNotesCount))
	}
	if cmd.SaveDataOption != "" {
		env = append(env, "MCP_SAVE_DATA_OPTION="+cmd.SaveDataOption)
	}
	if cmd.StartPage > 0 {
		env = append(env, fmt.Sprintf("MCP_START_PAGE=%d", cmd.StartPage))
	}
	c.Env = env

	var stdout bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stdout

	if err := c.Start(); err != nil {
		_ = r.repo.UpdateResult(ctx, jobID, domain.StatusFailed, nil, "start subprocess: "+err.Error())
		return
	}
	r.procs[jobID] = c
	_ = r.repo.UpdateRunning(ctx, jobID, c.Process.Pid)

	// TODO(v1 增强): 轮询 data/checkpoints/{platform}_{mode}_*.json 读 crawled_note_list 算进度，刷新 UpdateProgress。

	_ = c.Wait()

	summary := parseCrawlSummary(stdout.String())
	status := domain.StatusCompleted
	errMsg := ""
	if s, ok := summary["status"].(string); ok && s == "failed" {
		status = domain.StatusFailed
		if e, ok := summary["error"].(string); ok {
			errMsg = e
		}
	} else if c.ProcessState != nil && !c.ProcessState.Success() {
		status = domain.StatusFailed
		errMsg = "subprocess exit code " + fmt.Sprint(c.ProcessState.ExitCode())
	}
	// 保留尾部日志
	tail := stdout.String()
	if len(tail) > 4096 {
		tail = tail[len(tail)-4096:]
	}
	_ = r.repo.UpdateStdoutTail(ctx, jobID, tail)
	_ = r.repo.UpdateResult(ctx, jobID, status, summary, errMsg)
	delete(r.procs, jobID)
}

// stop 终止子进程。signal: term(SIGTERM 默认)/kill(SIGKILL)。
func (r *runner) stop(jobID string, signal string) error {
	c, ok := r.procs[jobID]
	if !ok {
		return fmt.Errorf("job %s not running", jobID)
	}
	if signal == "kill" {
		return c.Process.Kill()
	}
	return c.Process.Signal(os.Interrupt)
}

// buildCLIArgs 拼 python main.py 的 CLI 参数（对齐 cmd_arg/arg.py 的 typer 定义）。
func buildCLIArgs(cmd StartCrawlerCommand) []string {
	args := []string{
		"--platform", cmd.Platform,
		"--type", cmd.CrawlerType,
	}
	if cmd.Keywords != "" {
		args = append(args, "--keywords", cmd.Keywords)
	}
	if len(cmd.URLs) > 0 {
		args = append(args, "--urls", strings.Join(cmd.URLs, ","))
	}
	if cmd.EnableComments {
		args = append(args, "--enable_comments")
	} else {
		args = append(args, "--no-enable_comments")
	}
	if cmd.EnableSubComments {
		args = append(args, "--enable_sub_comments")
	}
	if cmd.EnableCheckpoint {
		args = append(args, "--enable_checkpoint")
	} else {
		args = append(args, "--no-enable_checkpoint")
	}
	if cmd.CheckpointID != "" {
		args = append(args, "--checkpoint_id", cmd.CheckpointID)
	}
	if cmd.HotSearchMaxItems > 0 {
		args = append(args, "--hot_search_max_items", fmt.Sprint(cmd.HotSearchMaxItems))
	}
	if cmd.HotSearchPerItemCount > 0 {
		args = append(args, "--hot_search_per_item_count", fmt.Sprint(cmd.HotSearchPerItemCount))
	}
	return args
}

// parseCrawlSummary 从 stdout 解析 ---CRAWL_SUMMARY: {...}---。
func parseCrawlSummary(out string) map[string]any {
	m := matchLast(crawlSummaryRe, out)
	if m == nil {
		return nil
	}
	var summary map[string]any
	if err := json.Unmarshal([]byte(m[1]), &summary); err != nil {
		return nil
	}
	return summary
}

func matchLast(re *regexp.Regexp, s string) []string {
	matches := re.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	return matches[len(matches)-1]
}
