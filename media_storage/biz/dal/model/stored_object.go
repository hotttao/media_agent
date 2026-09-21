package model

import "time"

type StoredObject struct {
	ObjectPath  string     `gorm:"column:object_path;primaryKey"`
	Namespace   string     `gorm:"column:namespace;not null"`
	Driver      string     `gorm:"column:driver;not null"`
	BackendKey  string     `gorm:"column:backend_key;not null"`
	ContentType string     `gorm:"column:content_type;not null"`
	SizeBytes   int64      `gorm:"column:size_bytes;not null"`
	Checksum    string     `gorm:"column:checksum;not null"`
	State       string     `gorm:"column:state;not null"`
	DeleteAfter *time.Time `gorm:"column:delete_after"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (StoredObject) TableName() string { return "media_storage.stored_objects" }
