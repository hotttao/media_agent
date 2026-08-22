package model

import "time"

type Tag struct {
	ID, WorkspaceID, Name, Color, CreatedBy string
	Version                                 int64
	CreatedAt, UpdatedAt                    time.Time
}

func (Tag) TableName() string { return "tags" }

type ResourceTag struct {
	TagID, WorkspaceID, ResourceType, ResourceID string
	CreatedAt                                    time.Time
}

func (ResourceTag) TableName() string { return "resource_tags" }
