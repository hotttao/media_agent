package model

import (
	"gorm.io/datatypes"
	"time"
)

type RuntimeEnvironment struct {
	ID, Name, Endpoint   string
	Capacity             datatypes.JSON
	Status               string
	Version              int64
	CreatedAt, UpdatedAt time.Time
}

func (RuntimeEnvironment) TableName() string { return "runtime_environments" }

type WorkspacePlacement struct {
	WorkspaceID               string `gorm:"primaryKey"`
	EnvironmentID             *string
	Status, Reason, VolumeRef string
	Version                   int64
	UpdatedAt                 time.Time
}

func (WorkspacePlacement) TableName() string { return "workspace_placements" }
