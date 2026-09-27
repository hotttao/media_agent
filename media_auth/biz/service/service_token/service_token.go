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
	"sort"
	"strings"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainst "media_agent/media_auth/biz/domain/service_token"
	policyst "media_agent/media_auth/biz/policy/service_token"
	"media_agent/media_auth/biz/shared/token"
)

// Service 是 service_token 用例服务。
type Service struct {
	repo                domainst.TokenRepo
	membershipValidator func(context.Context, string, string) bool
}

// NewService 构造 service_token 用例服务。
func NewService(repo domainst.TokenRepo) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetMembershipValidator(validator func(context.Context, string, string) bool) {
	s.membershipValidator = validator
}

// TokenRecord 是 service 层返回的 token 记录（与 DTO 对齐，不含 hash/明文）。
type TokenRecord struct {
	ID             string
	Name           string
	CreatedBy      string
	CreatedAt      string // RFC3339
	RevokedAt      string // RFC3339，空串=未撤销
	IsPlatform     bool
	OrganizationID string
	Scopes         []string
	ExpiresAt      string
}

// CreateResult 是 CreateServiceToken 返回（含明文 token，仅本次可见）。
type CreateResult struct {
	ID             string
	Name           string
	Token          string // tos_xxx 明文，仅本次响应可见
	CreatedAt      string // RFC3339
	OrganizationID string
	Scopes         []string
	ExpiresAt      string
}

// 用例错误。
var (
	ErrNameRequired          = errors.New("name is required")
	ErrNotFound              = domainst.ErrNotFound
	ErrOrganizationRequired  = errors.New("organization_id is required when scopes are granted")
	ErrExpiryRequired        = errors.New("expires_at is required when scopes are granted")
	ErrInvalidScope          = errors.New("invalid service token scope")
	ErrNotOrganizationMember = errors.New("user is not an active organization member")
)

var workerScopes = map[string]bool{
	"workflow.worker.poll":      true,
	"workflow.worker.heartbeat": true,
	"workflow.worker.complete":  true,
	"workflow.worker.fail":      true,
}

// CreateServiceToken 创建 B2B service token（非 platform）。明文 token 仅本次返回。
func (s *Service) CreateServiceToken(ctx context.Context, userID, name string) (CreateResult, error) {
	if name == "" {
		return CreateResult{}, ErrNameRequired
	}
	plaintext, hash, err := token.GenerateTokenAndHash()
	if err != nil {
		return CreateResult{}, err
	}
	t, err := s.repo.Create(ctx, name, hash, userID, domainst.Access{})
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

func (s *Service) CreateServiceTokenWithAccess(ctx context.Context, userID, name, organizationID string, scopes []string, expiresAt time.Time) (CreateResult, error) {
	if name == "" {
		return CreateResult{}, ErrNameRequired
	}
	if len(scopes) == 0 {
		return s.CreateServiceToken(ctx, userID, name)
	}
	if organizationID == "" {
		return CreateResult{}, ErrOrganizationRequired
	}
	if expiresAt.IsZero() || !expiresAt.After(time.Now()) {
		return CreateResult{}, ErrExpiryRequired
	}
	scopes = uniqueScopes(scopes)
	if len(scopes) == 0 {
		return CreateResult{}, ErrInvalidScope
	}
	for _, scope := range scopes {
		if !workerScopes[scope] {
			return CreateResult{}, ErrInvalidScope
		}
	}
	if s.membershipValidator == nil || !s.membershipValidator(ctx, organizationID, userID) {
		return CreateResult{}, ErrNotOrganizationMember
	}
	plaintext, hash, err := token.GenerateTokenAndHash()
	if err != nil {
		return CreateResult{}, err
	}
	t, err := s.repo.Create(ctx, name, hash, userID, domainst.Access{
		OrganizationID: organizationID, Scopes: scopes, ExpiresAt: &expiresAt,
	})
	if err != nil {
		return CreateResult{}, err
	}
	return createResult(t, plaintext), nil
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
	return s.repo.Revoke(ctx, id, userID)
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
		ID:             t.ID,
		Name:           t.Name,
		CreatedBy:      t.CreatedBy,
		CreatedAt:      t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		IsPlatform:     t.IsPlatform,
		OrganizationID: t.OrganizationID,
		Scopes:         append([]string(nil), t.Scopes...),
	}
	if t.ExpiresAt != nil {
		r.ExpiresAt = t.ExpiresAt.Format(time.RFC3339)
	}
	if t.RevokedAt != nil {
		r.RevokedAt = t.RevokedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return r
}

func createResult(t domainst.Token, plaintext string) CreateResult {
	result := CreateResult{
		ID: t.ID, Name: t.Name, Token: plaintext, CreatedAt: t.CreatedAt.Format(time.RFC3339),
		OrganizationID: t.OrganizationID, Scopes: append([]string(nil), t.Scopes...),
	}
	if t.ExpiresAt != nil {
		result.ExpiresAt = t.ExpiresAt.Format(time.RFC3339)
	}
	return result
}

func uniqueScopes(scopes []string) []string {
	seen := make(map[string]bool, len(scopes))
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope != "" && !seen[scope] {
			seen[scope] = true
			result = append(result, scope)
		}
	}
	sort.Strings(result)
	return result
}
