// Package like 是 like 领域仓储的 dal 实现。
// 实现 biz/domain/like.Repo；唯一可 import biz/dal/query 的位置（当前用裸 GORM，未走 Gen）。
// 把 DB row model 转为 domain 对象，禁止把 GORM row model 返回给上层。
package like

import (
	"context"

	"gorm.io/gorm"

	"media_agent/media_example/biz/dal"
	"media_agent/media_example/biz/dal/model"
	domainlike "media_agent/media_example/biz/domain/like"
)

// NewRepo 构造 like 仓储。db 为 dal 层 *gorm.DB（非事务时用，事务时由 ctx 覆盖）。
func NewRepo(db *gorm.DB) domainlike.Repo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

// Save 持久化一次点赞。从 ctx 取连接（上层在事务内则加入事务）。
// 重复点赞由 uk_likes_user_post 唯一约束拒绝，返回 gorm.ErrDuplicatedKey。
func (r *repo) Save(ctx context.Context, l domainlike.Like) error {
	row := model.LikeRow{
		UserID: l.UserID,
		PostID: l.PostID,
	}
	return dal.FromContext(ctx, r.db).Create(&row).Error
}
