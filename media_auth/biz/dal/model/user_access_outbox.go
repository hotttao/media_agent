package model

import "time"

type UserAccessOutbox struct {
	ID          string     `gorm:"column:id;primaryKey"`
	UserID      string     `gorm:"column:user_id;not null"`
	AuthVersion int64      `gorm:"column:auth_version;not null"`
	EventType   string     `gorm:"column:event_type;not null"`
	Status      string     `gorm:"column:status;not null"`
	Attempts    int        `gorm:"column:attempts;not null"`
	AvailableAt time.Time  `gorm:"column:available_at;not null"`
	DeliveredAt *time.Time `gorm:"column:delivered_at"`
	LastError   *string    `gorm:"column:last_error"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null"`
}

func (UserAccessOutbox) TableName() string { return "media_auth.user_access_outbox" }
