// Package counter 是 counter 领域仓储的 dal 实现。
// 实现 biz/domain/counter.Repo；唯一可 import biz/dal/query 的位置（当前用裸 GORM，未走 Gen）。
// 把 DB row model 转为 domain 对象，禁止把 GORM row model 返回给上层。
package counter

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_example/biz/dal"
	"media_agent/media_example/biz/dal/model"
	domaincounter "media_agent/media_example/biz/domain/counter"
)

// NewRepo 构造 counter 仓储。db 为 dal 层 *gorm.DB（非事务时用，事务时由 ctx 覆盖）。
func NewRepo(db *gorm.DB) domaincounter.Repo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

// IncrLikes 把指定帖子的点赞数 +1。从 ctx 取连接（上层在事务内则加入事务）。
// 行不存在时 upsert 插入 likes_count=1；存在时 likes_count +1 + updated_at=now。
// 注意：pg 无 ON UPDATE 自动更新，updated_at 在此显式更新（mysql 时由 DB 自动更新）。
func (r *repo) IncrLikes(ctx context.Context, postID string) error {
	db := dal.FromContext(ctx, r.db)
	row := model.PostCounterRow{PostID: postID, LikesCount: 1}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "post_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"likes_count": gorm.Expr("post_counters.likes_count + 1"),
			"updated_at":  gorm.Expr("now()"),
		}),
	}).Create(&row).Error
}
