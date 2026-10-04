package model

import "time"

type PlatformToken struct {
	ID         string     `gorm:"column:id;primaryKey"`
	UserID     string     `gorm:"column:user_id"`
	TokenHash  []byte     `gorm:"column:token_hash;type:bytea"`
	Ciphertext []byte     `gorm:"column:ciphertext;type:bytea"`
	Nonce      []byte     `gorm:"column:nonce;type:bytea"`
	KeyVersion string     `gorm:"column:key_version"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	ExpiresAt  *time.Time `gorm:"column:expires_at"`
	RevokedAt  *time.Time `gorm:"column:revoked_at"`
}

func (PlatformToken) TableName() string { return "media_auth.platform_tokens" }
