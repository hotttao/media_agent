package model

import "time"

type Prompt struct {
	ID, OwnerUserID, Name, Content, Status string
	WorkspaceID                            *string
	Version                                int64
	CreatedAt, UpdatedAt                   time.Time
}

func (Prompt) TableName() string { return "prompts" }
