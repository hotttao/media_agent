package model

import (
	"time"

	"gorm.io/datatypes"
)

// Job 对应 PG jobs 表（crawl_job domain 持久化）。
// 主键 job_id（业务生成 job_{ts}_{rand}）；status 状态机见 crawl_job domain。
type Job struct {
	JobID         string         `gorm:"column:job_id;primaryKey"`
	Platform      string         `gorm:"column:platform;not null"`
	CrawlerType   string         `gorm:"column:crawler_type;not null"`
	Params        datatypes.JSON `gorm:"column:params;type:jsonb"`
	Status        string         `gorm:"column:status;not null;default:queued"`
	Pid           *int           `gorm:"column:pid"`
	CheckpointID  string         `gorm:"column:checkpoint_id"`
	Progress      datatypes.JSON `gorm:"column:progress;type:jsonb"`
	ResultSummary datatypes.JSON `gorm:"column:result_summary;type:jsonb"`
	Error         string         `gorm:"column:error"`
	StdoutTail    string         `gorm:"column:stdout_tail"`
	QueuedAt      time.Time      `gorm:"column:queued_at;not null"`
	StartedAt     *time.Time     `gorm:"column:started_at"`
	FinishedAt    *time.Time     `gorm:"column:finished_at"`
}

func (Job) TableName() string { return `"media-crawler".jobs` }
