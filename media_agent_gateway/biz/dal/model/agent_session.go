package model

import (
	"encoding/json"
	"time"
)

type AgentSession struct {
	ID, UserID, WorkspaceID, AgentID, AgentType, Title, RuntimeSessionID, Status, ChatStatus string
	ContextVersion                                                                           int64
	CreatedAt, UpdatedAt                                                                     time.Time
}

func (AgentSession) TableName() string { return "media_agent_gateway.agent_sessions" }

type Turn struct {
	ID, SessionID, IdempotencyKey, Status string
	StartedAt                             time.Time
	FinishedAt                            *time.Time
}

func (Turn) TableName() string { return "media_agent_gateway.turns" }

type Message struct {
	ID, SessionID, TurnID, Role string
	Content                     json.RawMessage `gorm:"type:jsonb"`
	Sequence                    int64
	CreatedAt                   time.Time
}

func (Message) TableName() string { return "media_agent_gateway.messages" }

type UniversalEvent struct {
	ID, SessionID, TurnID, Type string
	Payload                     json.RawMessage `gorm:"type:jsonb"`
	Sequence                    int64
	CreatedAt                   time.Time
}

func (UniversalEvent) TableName() string { return "media_agent_gateway.universal_events" }
