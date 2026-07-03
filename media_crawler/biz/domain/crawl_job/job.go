package crawl_job

import "time"

// Status 抓取任务状态机值对象。
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusRejected  Status = "rejected"
)

// IsTerminal 终态判定（completed/failed/cancelled/rejected）。
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusRejected:
		return true
	}
	return false
}

// Job 抓取任务实体（crawl_job domain 核心，不带 ORM/JSON tag）。
// 主键 JobID 由应用生成 job_{ts}_{rand}；状态机见 crawl_job domain 文档。
type Job struct {
	JobID         string
	Platform      string
	CrawlerType   string
	Params        map[string]any // 入参回显（keywords/urls/enable_comments 等）
	Status        Status
	Pid           *int
	CheckpointID  string
	Progress      map[string]any // crawled_count/total_target/current_keyword/current_page/percent
	ResultSummary map[string]any // CRAWL_SUMMARY：platform/crawler_type/status/error
	Error         string
	StdoutTail    string
	QueuedAt      time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}
