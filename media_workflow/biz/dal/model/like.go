// Package model 存放 GORM row model（DB 行模型，承载 GORM tag）。
// 禁止被 service/workflow/policy/domain 直接 import；仅 dal/repo 使用。
// schema 由 biz/dal/migrations 定义，row model 与之对齐；schema 变更 → 加 migration → 重跑 GORM Gen。
package model

import "time"

// LikeRow 是 likes 表的 GORM row model，对应 like 领域实体的持久化形态。
type LikeRow struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    string    `gorm:"column:user_id;size:64;not null"`
	PostID    string    `gorm:"column:post_id;size:64;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName 固定表名（与 migration 一致）。
func (LikeRow) TableName() string { return "likes" }
