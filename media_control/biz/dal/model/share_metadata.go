package model

import "time"

type ShareMetadata struct {
	ID, WorkspaceID, ResourceType, ResourceID, OwnerUserID, Status string
	CreatedAt, RevokedAt                                           *time.Time
}

func (ShareMetadata) TableName() string { return "share_metadata" }
