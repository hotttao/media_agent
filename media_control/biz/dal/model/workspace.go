package model

import "time"

type Workspace struct {
	ID, OwnerUserID, Name, DesiredState string
	Version                             int64
	CreatedAt, UpdatedAt                time.Time
}

func (Workspace) TableName() string { return "workspaces" }

type AgentConfiguration struct {
	ID, WorkspaceID, AgentID, AgentType string
	EncryptedAPIKey                     []byte
	Version                             int64
	CreatedAt, UpdatedAt                time.Time
}

func (AgentConfiguration) TableName() string { return "agent_configurations" }
