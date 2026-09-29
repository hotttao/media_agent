package model

import (
	"time"

	"github.com/lib/pq"
)

type APIToken struct {
	ID               string         `gorm:"column:id;primaryKey"`
	TalosKeyID       *string        `gorm:"column:talos_key_id;unique"`
	SubjectUserID    *string        `gorm:"column:subject_user_id"`
	ActorType        string         `gorm:"column:actor_type;not null"`
	ActorID          string         `gorm:"column:actor_id;not null"`
	OrganizationID   *string        `gorm:"column:organization_id"`
	Name             string         `gorm:"column:name;not null"`
	Scopes           pq.StringArray `gorm:"column:scopes;type:text[];not null"`
	Status           string         `gorm:"column:status;not null"`
	ExpiresAt        *time.Time     `gorm:"column:expires_at"`
	CreatedBy        string         `gorm:"column:created_by;not null"`
	CreatedAt        time.Time      `gorm:"column:created_at;not null"`
	RotatedAt        *time.Time     `gorm:"column:rotated_at"`
	RevokedAt        *time.Time     `gorm:"column:revoked_at"`
	IssuanceDeadline *time.Time     `gorm:"column:issuance_deadline"`
	ClientRequestKey *string        `gorm:"column:client_request_key"`
}

func (APIToken) TableName() string { return "media_auth.api_tokens" }
