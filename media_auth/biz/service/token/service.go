// Package token 是 internal token 用例服务（VerifyToken RPC 的业务实现）。
//
// 不新建 domain：复用 service_token domain 的 TokenRepo.GetByHash + auth domain 的 UserRepo.FindByID
// （spec §2.2）。供下游服务验 opaque Bearer service token；明文 token 不出 media_auth 边界
// （service 内部 sha256 后查表）。
//
// 禁止 import Hertz / hertz_gen / biz/dal / 其它 service 包（同层隔离）。
package token

import (
	"context"
	"errors"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainst "media_agent/media_auth/biz/domain/service_token"
	"media_agent/media_auth/biz/shared/token"
)

// Service 是 internal token 用例服务。
type Service struct {
	tokenRepo domainst.TokenRepo
	userRepo  domainauth.UserRepo
}

// NewService 构造 internal token 服务。
// tokenRepo 来自 service_token domain；userRepo 来自 auth domain（取 display_name 填 name 字段）。
func NewService(tokenRepo domainst.TokenRepo, userRepo domainauth.UserRepo) *Service {
	return &Service{tokenRepo: tokenRepo, userRepo: userRepo}
}

// VerifyResult 是 VerifyToken 返回（与 token.proto VerifyTokenResponse 对齐）。
type VerifyResult struct {
	Active    bool
	Sub       string // = service_tokens.created_by
	Username  string // svc:{name}
	Name      string // user.display_name
	Role      string // user
	TokenType string // service_token
}

// VerifyToken 把明文 service token 解析为 Subject。
//
// 流程：sha256(明文) → GetByHash → 命中且未撤销 → {active, sub, username=svc:{name}, name, role=user, token_type=service_token}；
// 未命中/已撤销 → {active:false}。明文 token 永不出 media_auth 边界（仅 hash 后查表）。
func (s *Service) VerifyToken(ctx context.Context, plaintext string) (VerifyResult, error) {
	if plaintext == "" {
		return VerifyResult{Active: false}, nil
	}
	hash := token.HashToken(plaintext)
	t, err := s.tokenRepo.GetByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, domainst.ErrNotFound) {
			return VerifyResult{Active: false}, nil
		}
		return VerifyResult{}, err
	}
	// 取 user display_name 作 name 字段（缺失则空）。
	name := ""
	if u, err := s.userRepo.FindByID(ctx, t.CreatedBy); err == nil {
		name = u.DisplayName
	}
	return VerifyResult{
		Active:    true,
		Sub:       t.CreatedBy,
		Username:  "svc:" + t.Name,
		Name:      name,
		Role:      "user",
		TokenType: "service_token",
	}, nil
}
