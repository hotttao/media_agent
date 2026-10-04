package model

import "time"

type PlatformTokenAudit struct {
	ID            string     `gorm:"column:id;primaryKey"`
	UserID        string     `gorm:"column:user_id"`
	CredentialID  string     `gorm:"column:credential_id"`
	Operation     string     `gorm:"column:operation"`
	CallerService string     `gorm:"column:caller_service"`
	Purpose       string     `gorm:"column:purpose"`
	OccurredAt    time.Time  `gorm:"column:occurred_at"`
	DeliveredAt   *time.Time `gorm:"column:delivered_at"`
	LeaseID       *string    `gorm:"column:lease_id"`
	LeaseUntil    *time.Time `gorm:"column:lease_until"`
	Attempts      int        `gorm:"column:attempts"`
	NextAttemptAt time.Time  `gorm:"column:next_attempt_at"`
}

func (PlatformTokenAudit) TableName() string { return "media_auth.platform_token_audit_outbox" }
