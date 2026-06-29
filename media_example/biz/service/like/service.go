// Package like 是 like 用例服务层。
// 接受 command，编排 domain 完成用例，禁止 import Hertz / hertz_gen / biz/dal / biz/infra。
//
// 事务约定（architecture.md「事务 TxRunner + ctx 传递」）：
// 单 domain 用例在 service 顶部 RunInTx；service 内部方法不开事务只透传 ctx。
// like 用例本身是单条插入，事务由上层 workflow（点赞 = like + counter 同事务）持有，
// 故本 service 不开事务，直接用传入 ctx 调 repo。
package like

import (
	"context"

	domainlike "media_agent/media_example/biz/domain/like"
)

// Service 是 like 用例服务。
type Service struct {
	repo domainlike.Repo
}

// NewService 构造 like 用例服务。
func NewService(repo domainlike.Repo) *Service {
	return &Service{repo: repo}
}

// LikeCommand 是 Like 用例的入参。
type LikeCommand struct {
	UserID string
	PostID string
}

// Like 执行点赞用例：构造 domain Like 并持久化。
// 不开事务——透传 ctx，由上层 workflow 持有事务时自动加入。
func (s *Service) Like(ctx context.Context, cmd LikeCommand) error {
	l := domainlike.NewLike(cmd.UserID, cmd.PostID)
	return s.repo.Save(ctx, l)
}
