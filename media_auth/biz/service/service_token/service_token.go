// Package servicetoken 是 service_token 用例服务层（单 domain 编排）。
//
// 接受 command，编排 domain/policy/repo 完成用例；禁止 import Hertz / hertz_gen / biz/dal。
// 对齐 control-plane routes/service-tokens.ts + lib/service-token.ts + services/db/shares.ts。
//
// 同时实现 domain/auth.PlatformTokenEnsurer：auth service 经 SetPlatformTokenEnsurer 注入本服务，
// 登录成功后 fire-and-forget ensure platform token。
package servicetoken

import (
	"context"
	"errors"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainst "media_agent/media_auth/biz/domain/service_token"
	policyst "media_agent/media_auth/biz/policy/service_token"
	"media_agent/media_auth/biz/shared/token"
)

// Service 是 service_token 用例服务。
type Service struct {
	repo domainst.TokenRepo
}

// NewService 构造 service_token 用例服务。
func NewService(repo domainst.TokenRepo) *Service {
	return &Service{repo: repo}
}

// TokenRecord 是 service 层返回的 token 记录（与 DTO 对齐，不含 hash/明文）。
type TokenRecord struct {
	ID         string
	Name       string
	CreatedBy  string
	CreatedAt  string // RFC3339
	RevokedAt  string // RFC3339，空串=未撤销
	IsPlatform bool
}

// CreateResult 是 CreateServiceToken 返回（含明文 token，仅本次可见）。
type CreateResult struct {
	ID        string
	Name      string
	Token     string // tos_xxx 明文，仅本次响应可见
	CreatedAt string // RFC3339
}

// 用例错误。
var (
	ErrNameRequired = errors.New("name is required")
	ErrNotFound     = domainst.ErrNotFound
)

// CreateServiceToken 创建 B2B service token（非 platform）。明文 token 仅本次返回。
func (s *Service) CreateServiceToken(ctx context.Context, userID, name string) (CreateResult, error) {
	if name == "" {
		return CreateResult{}, ErrNameRequired
	}
	plaintext, hash, err := token.GenerateTokenAndHash()
	if err != nil {
		return CreateResult{}, err
	}
	t, err := s.repo.Create(ctx, name, hash, userID)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{
		ID:        t.ID,
		Name:      t.Name,
		Token:     plaintext,
		CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// ListServiceTokens 列 user 的全部未撤销 token（不含 hash/明文）。
func (s *Service) ListServiceTokens(ctx context.Context, userID string) ([]TokenRecord, error) {
	rows, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]TokenRecord, 0, len(rows))
	for _, t := range rows {
		out = append(out, toRecord(t))
	}
	return out, nil
}

// RevokeServiceToken 撤销 token（仅本人非 platform；不属本人/已撤销/platform → 404）。
func (s *Service) RevokeServiceToken(ctx context.Context, userID, id string) error {
	return s.repo.Revoke(ctx, id) // repo WHERE 含 is_platform=false；不属本人/不存在 → 0 行 → ErrNotFound
}

// ── 内部能力 ──────────────────────────────────────────────────────────

// EnsurePlatformToken 实现 domain/auth.PlatformTokenEnsurer。
// 确保 user 拥有 active platform token（不存在则建）。仅副作用，不返回明文。
func (s *Service) EnsurePlatformToken(ctx context.Context, userID string) error {
	_, err := s.repo.EnsurePlatformToken(ctx, userID)
	return err
}

// GetServiceTokenByHash 按 hash 查未撤销 token（供 token service 的 VerifyToken RPC）。
func (s *Service) GetServiceTokenByHash(ctx context.Context, tokenHash string) (domainst.Token, error) {
	return s.repo.GetByHash(ctx, tokenHash)
}

// GetPlatformToken 取 user 的 active platform token 明文（供 control_plane agent 回调）。
// 不存在返回 ""。
func (s *Service) GetPlatformToken(ctx context.Context, userID string) (string, error) {
	return s.repo.GetPlatformToken(ctx, userID)
}

// 编译期断言：Service 实现 PlatformTokenEnsurer。
var _ domainauth.PlatformTokenEnsurer = (*Service)(nil)

// CanRevoke 暴露 policy 决策供潜在外部校验（当前 repo 已在 SQL 层强制，保留供单测）。
var _ = policyst.CanRevoke

// toRecord 把 domain Token 转 service 层 TokenRecord（DTO 友好，丢弃 hash/明文）。
func toRecord(t domainst.Token) TokenRecord {
	r := TokenRecord{
		ID:         t.ID,
		Name:       t.Name,
		CreatedBy:  t.CreatedBy,
		CreatedAt:  t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		IsPlatform: t.IsPlatform,
	}
	if t.RevokedAt != nil {
		r.RevokedAt = t.RevokedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return r
}
