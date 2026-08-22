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

type BatchRunOutbox struct {
	ID, BatchRunID, Action            string
	Payload                           string `gorm:"type:jsonb"`
	Attempts                          int32
	AvailableAt, CreatedAt, UpdatedAt time.Time
	DeliveredAt                       *time.Time
	LastError                         *string
}

func (BatchRunOutbox) TableName() string { return "media_agent_gateway.batch_run_outbox" }

type BatchRunInbox struct {
	EventID, BatchRunID, ItemKey string
	CreatedAt                    time.Time
}

func (BatchRunInbox) TableName() string { return "media_agent_gateway.batch_run_inbox" }
