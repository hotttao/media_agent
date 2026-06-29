// Package workflow 存放跨 domain 业务流程编排。
// 禁止 import handler / model / 原始 SQL / dal query / dal model（见 architecture.md 依赖规则）。
//
// like_flow：点赞流程 = like domain 插入点赞记录 + counter domain 帖子点赞数 +1，
// 二者须同事务，避免「记了点赞但计数没涨」的不一致。
// 事务由 workflow 持有 TxRunner.RunInTx 开启（跨 domain 事务开启点），经 ctx 透传给两个 service。
package workflow

import (
	"context"

	"media_agent/media_example/biz/dal"
	"media_agent/media_example/biz/service/counter"
	"media_agent/media_example/biz/service/like"
)

// LikeFlow 是点赞流程编排：协调 like + counter 两个 domain 的 service。
type LikeFlow struct {
	tx         dal.TxRunner
	likeSvc    *like.Service
	counterSvc *counter.Service
}

// NewLikeFlow 构造点赞流程。tx 可为 nil（database 禁用场景，仅用于装配不实际执行）。
func NewLikeFlow(tx dal.TxRunner, likeSvc *like.Service, counterSvc *counter.Service) *LikeFlow {
	return &LikeFlow{tx: tx, likeSvc: likeSvc, counterSvc: counterSvc}
}

// LikeCommand 是点赞流程的入参。
type LikeCommand struct {
	UserID string
	PostID string
}

// Like 执行点赞：在单个 DB 事务内先记点赞关系、再加帖子点赞数；任一失败整体回滚。
//
// 流程：tx.RunInTx 开启事务 → likeSvc.Like(txCtx) → counterSvc.IncrLikes(txCtx)。
// tx 经 ctx 向下传递，两个 repo 自动加入同一事务。service 之间互不 import（同层隔离）。
func (f *LikeFlow) Like(ctx context.Context, cmd LikeCommand) error {
	return f.tx.RunInTx(ctx, func(txCtx context.Context) error {
		if err := f.likeSvc.Like(txCtx, like.LikeCommand{
			UserID: cmd.UserID,
			PostID: cmd.PostID,
		}); err != nil {
			return err // 自动回滚
		}
		return f.counterSvc.IncrLikes(txCtx, counter.IncrLikesCommand{
			PostID: cmd.PostID,
		}) // 任一失败整体回滚
	})
}
