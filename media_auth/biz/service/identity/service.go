// Package identity 是 identity 用例服务层（单 domain 编排）。
//
// 接受 command，编排 domain/repo 完成用例；禁止 import Hertz / hertz_gen / biz/dal。
// 对齐 control-plane services/db/identities.ts + routes/wecom-auth.ts(identity 段)。
//
// 同时实现 domain/auth.AuthSourceResolver：auth service 经 SetAuthSourceResolver 注入本服务，
// 推导无密码账号的 wecom/ldap 来源（spec §5.1 修正 control-plane 对企微账号误报 ldap）。
package identity

import (
	"context"
	"encoding/json"
	"errors"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainidentity "media_agent/media_auth/biz/domain/identity"
)

// Service 是 identity 用例服务。
type Service struct {
	repo domainidentity.IdentityRepo
}

// NewService 构造 identity 用例服务。
func NewService(repo domainidentity.IdentityRepo) *Service {
	return &Service{repo: repo}
}

// Identity 是 service 层返回的身份绑定（与 DTO 对齐，不含 metadata）。
type Identity struct {
	Provider    string
	ExternalID  string
	DisplayName string
	CreatedAt   string // RFC3339
}

// 用例错误（handler 据此映射 HTTP 状态）。
var (
	ErrNotFound     = domainidentity.ErrNotFound
	ErrAlreadyBound = domainidentity.ErrAlreadyBound
)

// ListIdentities 列当前 user 的全部外部身份绑定。
func (s *Service) ListIdentities(ctx context.Context, userID string) ([]Identity, error) {
	rows, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Identity, 0, len(rows))
	for _, r := range rows {
		out = append(out, toIdentity(r))
	}
	return out, nil
}

// DeleteIdentity 删 (user, provider) 绑定。0 行 → ErrNotFound（handler 404）。
func (s *Service) DeleteIdentity(ctx context.Context, userID, provider string) error {
	return s.repo.Delete(ctx, userID, domainidentity.Provider(provider))
}

// ── 内部能力（供 wecom workflow 调用） ─────────────────────────────────

// FindUserIDentity 按 (provider, external_id) 查所属 user_id（wecom login 用）。
// 不存在返回 ""（login 流程据此判 not_bound）。
func (s *Service) FindUserIDentity(ctx context.Context, provider, externalID string) (string, error) {
	id, err := s.repo.FindByExternal(ctx, domainidentity.Provider(provider), externalID)
	if err != nil {
		if errors.Is(err, domainidentity.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return id.UserID, nil
}

// BindIdentity 绑定/更新 (user, provider) 的外部身份（wecom bind 用）。
//
// 流程（对齐 spec §5.2）：
//  1. 先查 (provider, external_id) 是否已绑别人 → ErrAlreadyBound；
//  2. Upsert（ON CONFLICT (user_id, provider) DO UPDATE external_id/display_name/metadata）。
//
// displayName 空串=不设；metadata 为 raw JSON（可空）。
func (s *Service) BindIdentity(ctx context.Context, userID, provider, externalID, displayName string, metadata json.RawMessage) error {
	// 1. 预检：(provider, external_id) 是否已属别人。
	if existing, err := s.repo.FindByExternal(ctx, domainidentity.Provider(provider), externalID); err == nil {
		if existing.UserID != userID {
			return ErrAlreadyBound
		}
		// 同一 user 的既有绑定 → 走 Upsert 更新。
	} else if !errors.Is(err, domainidentity.ErrNotFound) {
		return err
	}
	// 2. Upsert。
	_, err := s.repo.Upsert(ctx, domainidentity.Identity{
		UserID:      userID,
		Provider:    domainidentity.Provider(provider),
		ExternalID:  externalID,
		DisplayName: displayName,
		Metadata:    metadata,
	})
	return err
}

// ── AuthSourceResolver 实现（供 auth service 注入） ───────────────────

// ResolveByUserID 实现 domain/auth.AuthSourceResolver。
//
// 推导无密码账号的外部身份来源：有 wecom identity → "wecom"；否则 ""（auth 回退 ldap）。
// 仅查 (user_id, wecom) 唯一索引，O(1)。
func (s *Service) ResolveByUserID(ctx context.Context, userID string) string {
	if _, err := s.repo.FindByUserProvider(ctx, userID, domainidentity.ProviderWeCom); err == nil {
		return string(domainidentity.ProviderWeCom)
	}
	return ""
}

// 编译期断言：Service 实现 AuthSourceResolver。
var _ domainauth.AuthSourceResolver = (*Service)(nil)

// toIdentity 把 domain Identity 转 service 层 Identity（DTO 友好，丢弃 metadata）。
func toIdentity(r domainidentity.Identity) Identity {
	return Identity{
		Provider:    string(r.Provider),
		ExternalID:  r.ExternalID,
		DisplayName: r.DisplayName,
		CreatedAt:   r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
