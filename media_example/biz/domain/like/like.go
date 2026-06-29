// Package like 存放 like（点赞）领域的稳定业务核心：实体、值对象、仓储接口。
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM SDK。
package like

import "context"

// Like 是一次点赞的领域实体（谁给哪个帖子点了赞）。
type Like struct {
	UserID string
	PostID string
}

// NewLike 构造一次点赞。userID / postID 为空时由上层校验拦截（repo 唯一约束兜底）。
func NewLike(userID, postID string) Like {
	return Like{UserID: userID, PostID: postID}
}

// Repo 是 like 领域的仓储接口（由 dal/repo/like 实现）。
//
// 仓储方法收 context.Context：上层在事务内时 ctx 携带 tx（见 dal.FromContext），
// repo 据此加入事务；不在事务内时用默认连接。仓储自身不开事务。
type Repo interface {
	// Save 持久化一次点赞。重复点赞由 DB 唯一约束（uk_likes_user_post）拒绝。
	Save(ctx context.Context, l Like) error
}
