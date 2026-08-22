package model

import "time"

type AgentSession struct {
	ID, UserID, WorkspaceID, AgentID, AgentType, Title, Status string
	ContextVersion                                             int64
	CreatedAt, UpdatedAt                                       time.Time
}

func (AgentSession) TableName() string { return "media_agent_gateway.agent_sessions" }
