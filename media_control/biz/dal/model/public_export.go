package model

import "time"

type PublicExport struct {
	ID, WorkspaceID, ResourceType, ResourceID, ObjectPath, Status, CreatedBy string
	CreatedAt                                                                time.Time
}

func (PublicExport) TableName() string { return "public_exports" }
