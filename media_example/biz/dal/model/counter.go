// Package model 存放 GORM row model（DB 行模型，承载 GORM tag）。
// 禁止被 service/workflow/policy/domain 直接 import；仅 dal/repo 使用。
// schema 由 biz/dal/migrations 定义，row model 与之对齐；schema 变更 → 加 migration → 重跑 GORM Gen。
package model

import "time"

// PostCounterRow 是 post_counters 表的 GORM row model，对应 counter 领域实体的持久化形态。
type PostCounterRow struct {
	PostID     string    `gorm:"column:post_id;primaryKey"`
	LikesCount uint64    `gorm:"column:likes_count;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

// TableName 固定表名（与 migration 一致）。
func (PostCounterRow) TableName() string { return "post_counters" }
