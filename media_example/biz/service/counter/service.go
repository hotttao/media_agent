// Package counter 是 counter 用例服务层。
// 接受 command，编排 domain 完成用例，禁止 import Hertz / hertz_gen / biz/dal / biz/infra。
//
// 事务约定（architecture.md「事务 TxRunner + ctx 传递」）：
// 单 domain 用例在 service 顶部 RunInTx；service 内部方法不开事务只透传 ctx。
// counter 用例本身是单条 +1，事务由上层 workflow（点赞 = like + counter 同事务）持有，
// 故本 service 不开事务，直接用传入 ctx 调 repo。
package counter

import (
	"context"

	domaincounter "media_agent/media_example/biz/domain/counter"
)

// Service 是 counter 用例服务。
type Service struct {
	repo domaincounter.Repo
}

// NewService 构造 counter 用例服务。
func NewService(repo domaincounter.Repo) *Service {
	return &Service{repo: repo}
}

// IncrLikesCommand 是 IncrLikes 用例的入参。
type IncrLikesCommand struct {
	PostID string
}

// IncrLikes 把指定帖子点赞数 +1。
// 不开事务——透传 ctx，由上层 workflow 持有事务时自动加入。
func (s *Service) IncrLikes(ctx context.Context, cmd IncrLikesCommand) error {
	return s.repo.IncrLikes(ctx, cmd.PostID)
}
