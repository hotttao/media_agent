package model

import "time"

type APITokenRotation struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TokenID   string    `gorm:"column:token_id;not null"`
	OldKeyID  string    `gorm:"column:old_key_id;not null"`
	ActorID   string    `gorm:"column:actor_id;not null"`
	Status    string    `gorm:"column:status;not null"`
	Deadline  time.Time `gorm:"column:deadline;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
}

func (APITokenRotation) TableName() string { return "media_auth.api_token_rotations" }
