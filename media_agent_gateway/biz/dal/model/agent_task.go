package model

import "time"

type AgentTask struct {
	ID, WorkspaceID, RequestedBy             string
	SessionID                                *string
	Source, SourceResourceID, IdempotencyKey string
	Input                                    string `gorm:"type:jsonb"`
	Status                                   string
	Attempt                                  int32
	TurnID                                   *string
	ResultRef                                *string `gorm:"type:jsonb"`
	ErrorCode, ErrorMessage                  *string
	CreatedAt, UpdatedAt                     time.Time
	StartedAt, FinishedAt                    *time.Time
}

func (AgentTask) TableName() string { return "media_agent_gateway.agent_tasks" }

type AgentTaskOutbox struct {
	ID, AggregateID, EventType        string
	Payload                           string `gorm:"type:jsonb"`
	Attempts                          int32
	AvailableAt, CreatedAt, UpdatedAt time.Time
	DeliveredAt                       *time.Time
	LastError                         *string
}

func (AgentTaskOutbox) TableName() string { return "media_agent_gateway.agent_task_outbox" }
