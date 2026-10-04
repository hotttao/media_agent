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
	"strings"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainst "media_agent/media_auth/biz/domain/service_token"
	"media_agent/media_auth/biz/shared/token"
)

// Service 是 internal token 用例服务。
type Service struct {
	tokenRepo           domainst.TokenRepo
	userRepo            domainauth.UserRepo
	membershipValidator func(context.Context, string, string) bool
}

func (s *Service) SetMembershipValidator(validator func(context.Context, string, string) bool) {
	s.membershipValidator = validator
}

// NewService 构造 internal token 服务。
// tokenRepo 来自 service_token domain；userRepo 来自 auth domain（取 display_name 填 name 字段）。
func NewService(tokenRepo domainst.TokenRepo, userRepo domainauth.UserRepo) *Service {
	return &Service{tokenRepo: tokenRepo, userRepo: userRepo}
}

// VerifyResult 是 VerifyToken 返回（与 token.proto VerifyTokenResponse 对齐）。
type VerifyResult struct {
	Active         bool
	Sub            string // = service_tokens.created_by
	Username       string // svc:{name}
	Name           string // user.display_name
	Role           string // user
	TokenType      string // service_token
	CredentialID   string
	OrganizationID string
	Scope          string
	ExpiresAt      int64
	AuthSource     string
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
	if t.IsPlatform || !t.IsActiveAt(time.Now()) {
		return VerifyResult{Active: false}, nil
	}
	if len(t.Scopes) > 0 {
		if t.OrganizationID == "" || s.membershipValidator == nil || !s.membershipValidator(ctx, t.OrganizationID, t.CreatedBy) {
			return VerifyResult{Active: false}, nil
		}
	}
	// 取 user display_name 作 name 字段（缺失则空）。
	name := ""
	u, userErr := s.userRepo.FindByID(ctx, t.CreatedBy)
	if userErr != nil || !u.IsActive() {
		return VerifyResult{Active: false}, nil
	}
	if userErr == nil {
		name = u.DisplayName
	}
	result := VerifyResult{
		Active:         true,
		Sub:            t.CreatedBy,
		Username:       "svc:" + t.Name,
		Name:           name,
		Role:           "user",
		TokenType:      "service_token",
		CredentialID:   t.ID,
		OrganizationID: t.OrganizationID,
		Scope:          strings.Join(t.Scopes, " "),
		AuthSource:     "service_token",
	}
	if t.ExpiresAt != nil {
		result.ExpiresAt = t.ExpiresAt.Unix()
	}
	return result, nil
}

// ValidateSession 让旧 JWT 在用户软删除或 auth_version 递增后立即失效。
func (s *Service) ValidateSession(ctx context.Context, sess domainauth.Session) bool {
	u, err := s.userRepo.FindByID(ctx, sess.Sub)
	return err == nil && u.IsActive() && u.AuthVersion == sess.AuthVersion
}

func (s *Service) GetPlatformToken(ctx context.Context, userID string) (string, error) {
	// Retained only until legacy IDL removal. No reads, creates or fallback.
	return "", errors.New("legacy platform token endpoint disabled")
}
