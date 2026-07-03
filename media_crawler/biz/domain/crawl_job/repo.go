package crawl_job

import "context"

// JobRepo crawl_job domain 的仓库接口（由 dal/repo 实现）。
// repo 从 context 取连接（dal.FromContext），支持上层事务透传。
type JobRepo interface {
	// Save 创建 job（status=queued）。
	Save(ctx context.Context, job *Job) error
	// Get 取单个 job（含 progress/result_summary）。
	Get(ctx context.Context, jobID string) (*Job, error)
	// UpdateRunning 抢到串行锁后标记 running + 记 pid + started_at。
	UpdateRunning(ctx context.Context, jobID string, pid int) error
	// UpdateProgress 刷新进度（progress jsonb）。
	UpdateProgress(ctx context.Context, jobID string, progress map[string]any) error
	// UpdateResult 终态：status + result_summary + error + finished_at + pid=null。
	UpdateResult(ctx context.Context, jobID string, status Status, summary map[string]any, errMsg string) error
	// UpdateCheckpoint 回填运行时生成的 checkpoint_id。
	UpdateCheckpoint(ctx context.Context, jobID, checkpointID string) error
	// UpdateStdoutTail 刷新日志尾部。
	UpdateStdoutTail(ctx context.Context, jobID, tail string) error
	// List 分页列 job（platform/status 可选过滤）。
	List(ctx context.Context, platform, status string, limit, offset int) ([]*Job, int, error)
}
