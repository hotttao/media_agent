package model

import "time"

type Skill struct {
	ID, OwnerUserID, Name, ContentPath, Checksum, Status string
	WorkspaceID                                          *string
	Version                                              int64
	CreatedAt, UpdatedAt                                 time.Time
}

func (Skill) TableName() string { return "skills" }
