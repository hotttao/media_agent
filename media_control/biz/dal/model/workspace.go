package model

import "time"

type Workspace struct {
	ID, OrganizationID, TeamID, CreatedBy, OwnerUserID, Name, DesiredState string
	Version                                                                int64
	CreatedAt, UpdatedAt                                                   time.Time
}

func (Workspace) TableName() string { return "workspaces" }

type AgentConfiguration struct {
	ModelProtocol, ModelName, ModelBaseURL                 string
	ID, WorkspaceID, AgentID, AgentType, ModelConnectionID string
	EncryptedAPIKey                                        []byte
	Version                                                int64
	CreatedAt, UpdatedAt                                   time.Time
}

func (AgentConfiguration) TableName() string { return "agent_configurations" }
