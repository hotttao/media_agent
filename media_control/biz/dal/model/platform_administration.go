package model

import "time"

type PlatformSetting struct {
	Key       string
	Value     string `gorm:"type:jsonb"`
	UpdatedBy string
	Version   int64
	UpdatedAt time.Time
}

func (PlatformSetting) TableName() string { return "platform_settings" }

type PlatformSettingOutbox struct {
	ID, SettingKey, EventType string
	Payload                   string `gorm:"type:jsonb"`
	Version                   int64
	CreatedAt                 time.Time
	DeliveredAt               *time.Time
}

func (PlatformSettingOutbox) TableName() string { return "platform_setting_outbox" }
