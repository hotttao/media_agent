package model

import "time"

type WorkerIdentity struct {
	ID             string     `gorm:"column:id;primaryKey"`
	OwnerUserID    string     `gorm:"column:owner_user_id;not null"`
	OrganizationID string     `gorm:"column:organization_id;not null"`
	Name           string     `gorm:"column:name;not null"`
	Status         string     `gorm:"column:status;not null"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	DisabledAt     *time.Time `gorm:"column:disabled_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
}

func (WorkerIdentity) TableName() string { return "media_auth.worker_identities" }
