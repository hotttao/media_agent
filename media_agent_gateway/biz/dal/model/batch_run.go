package model

import "time"

type BatchRun struct {
	ID, WorkspaceID, RequestedBy, IdempotencyKey, Status string
	ItemCount, NextCursor                                int32
	CompletedCount, FailedCount, CancelledCount          int32
	CreatedAt, UpdatedAt                                 time.Time
	FinishedAt                                           *time.Time
}

func (BatchRun) TableName() string { return "media_agent_gateway.batch_runs" }

type BatchRunItem struct {
	BatchRunID, ItemKey, SessionID string
	AgentTaskID                    *string
	Input                          string `gorm:"type:jsonb"`
	Status                         string
	ErrorCode                      *string
	CreatedAt, UpdatedAt           time.Time
}

func (BatchRunItem) TableName() string { return "media_agent_gateway.batch_run_items" }
