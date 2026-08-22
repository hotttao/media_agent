package model

import (
	"time"

	"gorm.io/datatypes"
)

type AgentTemplate struct {
	ID, OwnerUserID, Name, Status string
	WorkspaceID                   *string
	Configuration                 datatypes.JSON
	Version                       int64
	CreatedAt, UpdatedAt          time.Time
}

func (AgentTemplate) TableName() string { return "agent_templates" }
