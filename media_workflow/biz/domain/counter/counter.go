// Package counter 存放 counter（帖子计数）领域的稳定业务核心：实体、仓储接口。
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM SDK。
package counter

import "context"

// Counter 是某个帖子的聚合计数（点赞数等）。
type Counter struct {
	PostID     string
	LikesCount uint64
}

// Repo 是 counter 领域的仓储接口（由 dal/repo/counter 实现）。
//
// 仓储方法收 context.Context：上层在事务内时 ctx 携带 tx（见 dal.FromContext），
// repo 据此加入事务；不在事务内时用默认连接。仓储自身不开事务。
type Repo interface {
	// IncrLikes 把指定帖子的点赞数 +1。帖子计数行不存在则插入（likes_count=1）。
	IncrLikes(ctx context.Context, postID string) error
}
