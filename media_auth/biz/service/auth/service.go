// Package auth 是 auth 用例服务层（单 domain 编排，单事务）。
//
// 接受 command，编排 domain/policy/外部 client 完成用例；禁止 import Hertz / hertz_gen / biz/dal。
// 事务：单 domain 用例，本 service 不开事务（透传 ctx）；登录涉及 upsert+last_login 若需事务可上提 workflow，
// 当前 upsert 已含 last_login 更新，单操作即一致。
//
// 对齐 control-plane routes/auth.ts + services/auth.ts。
package auth

import (
	"context"
	"errors"
	"log"

	domainauth "media_agent/media_auth/biz/domain/auth"
	policyauth "media_agent/media_auth/biz/policy/auth"
	"media_agent/media_auth/biz/shared/argon2"
	ldapclient "media_agent/media_auth/biz/shared/client/ldap"
)

// Service 是 auth 用例服务。
type Service struct {
	repo   domainauth.UserRepo
	signer domainauth.SessionSigner
	ldap   LdapAuthenticator
	// authSourceResolver 推导无密码账号的 wecom/ldap 来源（identity domain 实现）。
	// 当前 nil（identity 未实现），wecom 分支回退 ldap。
	resolver domainauth.AuthSourceResolver
	// platformTokenEnsurer 登录成功后 ensure platform service token（service_token domain 实现）。
	// nil 时跳过（service_token 未实现）。
	platformTokenEnsurer domainauth.PlatformTokenEnsurer
}

// LdapAuthenticator 是 LDAP 认证接口（*ldapclient.Client 实现它）。
// 抽成接口便于单测注入 fake，不依赖真实 LDAP 服务器。
type LdapAuthenticator interface {
	Authenticate(ctx context.Context, username, password string) (*ldapclient.LdapUser, error)
}

// NewService 构造 auth 用例服务。ldap 可为 nil（禁用 LDAP）。
func NewService(repo domainauth.UserRepo, signer domainauth.SessionSigner, ldap LdapAuthenticator) *Service {
	return &Service{repo: repo, signer: signer, ldap: ldap}
}

// SetAuthSourceResolver 注入 identity 提供的 auth_source 解析器（identity domain 完成后调用）。
func (s *Service) SetAuthSourceResolver(r domainauth.AuthSourceResolver) { s.resolver = r }

// SetPlatformTokenEnsurer 注入 service_token 提供的 platform token ensurer（service_token domain 完成后调用）。
func (s *Service) SetPlatformTokenEnsurer(e domainauth.PlatformTokenEnsurer) {
	s.platformTokenEnsurer = e
}

// ApiUser 是 service 层返回的用户档案（与 ApiUser DTO 对齐，不含 default_prompt_name，由网关补）。
type ApiUser struct {
	ID              string
	Username        string
	Role            string
	AuthSource      string
	DefaultPromptID string
	AutoEvolution   bool
}

// 用例错误（handler 据此映射 HTTP 状态）。
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled    = errors.New("this account cannot be used for login")
	ErrPasswordNotSet     = errors.New("account has no local password")
	ErrUsernameTaken      = domainauth.ErrUsernameTaken    // Register/CreateUser 用户名冲突（handler 409）
	ErrUsernameRequired   = policyauth.ErrUsernameRequired // Register/CreateUser 用户名空（handler 400）
	ErrPasswordTooShort   = policyauth.ErrPasswordTooShort // Register/ChangePassword 密码<6（handler 400）
	ErrPasswordSame       = policyauth.ErrPasswordSame     // ChangePassword 新旧相同（handler 400）
)

// Login 本地密码 / LDAP 双路登录。成功返回 ApiUser + session token（下 cookie 由 handler 做）。
//
// 流程（对齐 control-plane login）：
//  1. 先本地（argon2 verify password_hash），失败再 LDAP；
//  2. LDAP 命中则 upsert（已存在更新，不存在建号 password_hash 空）；
//  3. role=='system' → 403；
//  4. 本地登录成功更新 last_login_at；
//  5. 签 session（signer.Sign）；
//  6. fire-and-forget ensurePlatformToken（service_token domain 未实现，留日志占位）。
func (s *Service) Login(ctx context.Context, cmd LoginCommand) (ApiUser, string, error) {
	if cmd.Username == "" || cmd.Password == "" {
		return ApiUser{}, "", ErrInvalidCredentials
	}

	var u domainauth.User
	localOk := false

	// 1. 本地密码
	existing, err := s.repo.FindByUsername(ctx, cmd.Username)
	if err == nil && existing.HasPassword() {
		if err := argon2.Verify(cmd.Password, existing.PasswordHash); err == nil {
			localOk = true
			u = existing
		}
	}

	// 2. LDAP（未配置 ldap client 则跳过，直接判凭证失败）
	if !localOk {
		if s.ldap == nil {
			return ApiUser{}, "", ErrInvalidCredentials
		}
		ldapUser, err := s.ldap.Authenticate(ctx, cmd.Username, cmd.Password)
		if err != nil {
			return ApiUser{}, "", err
		}
		if ldapUser == nil {
			return ApiUser{}, "", ErrInvalidCredentials
		}
		// upsert：已存在更新，不存在建号。
		upserted, err := s.repo.Upsert(ctx, ldapUser.Username, ldapUser.Name, ldapUser.Email)
		if err != nil {
			return ApiUser{}, "", err
		}
		u = upserted
	}

	// 3. system 账号禁登
	if !policyauth.CanLogin(u) {
		return ApiUser{}, "", ErrAccountDisabled
	}

	// 4. 本地登录成功更新 last_login_at（fire-and-forget，失败不影响登录）。
	if localOk {
		if err := s.repo.UpdateLastLogin(ctx, u.ID); err != nil {
			log.Printf("auth: update last_login_at for %s: %v", u.ID, err)
		}
	}

	// 5. 签 session。
	sess := domainauth.Session{
		Sub:         u.ID,
		Username:    u.Username,
		Name:        u.DisplayName,
		Email:       u.Email,
		Role:        u.Role,
		AuthVersion: u.AuthVersion,
	}
	token, err := s.signer.Sign(sess)
	if err != nil {
		return ApiUser{}, "", err
	}

	// 6. ensurePlatformToken（fire-and-forget，失败仅日志，不影响登录）。
	if s.platformTokenEnsurer != nil {
		if err := s.platformTokenEnsurer.EnsurePlatformToken(ctx, u.ID); err != nil {
			log.Printf("auth: ensurePlatformToken for user %s: %v", u.ID, err)
		}
	} else {
		log.Printf("auth: platformTokenEnsurer not wired, skip ensurePlatformToken for %s", u.ID)
	}

	return toApiUser(u, s.resolver), token, nil
}

// Register 自服务注册：建本地账号（role=user，argon2 hash 密码）+ 签 session（自动登录）。
//
// 流程：校验入参 -> argon2 hash -> CreateUser(role=user) -> 签 session -> ensurePlatformToken。
// 用户名冲突 -> ErrUsernameTaken（handler 409）。成功返回 (ApiUser, sessionToken)。
func (s *Service) Register(ctx context.Context, cmd RegisterCommand) (ApiUser, string, error) {
	if err := policyauth.ValidateCreateUser(cmd.Username, cmd.Password); err != nil {
		return ApiUser{}, "", err
	}
	// displayName 缺省=username。
	displayName := cmd.Username
	if cmd.DisplayName != nil && *cmd.DisplayName != "" {
		displayName = *cmd.DisplayName
	}
	email := ""
	if cmd.Email != nil {
		email = *cmd.Email
	}
	hash, err := argon2.Hash(cmd.Password)
	if err != nil {
		return ApiUser{}, "", err
	}
	u, err := s.repo.CreateUser(ctx, cmd.Username, displayName, hash, email, "user")
	if err != nil {
		return ApiUser{}, "", err // ErrUsernameTaken 透传
	}
	// 签 session（自动登录）。
	sess := domainauth.Session{
		Sub:         u.ID,
		Username:    u.Username,
		Name:        u.DisplayName,
		Email:       u.Email,
		Role:        u.Role,
		AuthVersion: u.AuthVersion,
	}
	token, err := s.signer.Sign(sess)
	if err != nil {
		return ApiUser{}, "", err
	}
	// fire-and-forget ensure platform token。
	if s.platformTokenEnsurer != nil {
		go func() {
			if err := s.platformTokenEnsurer.EnsurePlatformToken(context.Background(), u.ID); err != nil {
				log.Printf("auth: ensurePlatformToken for %s: %v", u.ID, err)
			}
		}()
	}
	return toApiUser(u, s.resolver), token, nil
}

// GetMe 按 session.sub 查当前用户档案。
func (s *Service) GetMe(ctx context.Context, sess domainauth.Session) (ApiUser, error) {
	u, err := s.repo.FindByID(ctx, sess.Sub)
	if err != nil {
		return ApiUser{}, err
	}
	return toApiUser(u, s.resolver), nil
}

// UpdateMe 局部更新本人设置（目前仅 auto_evolution）。AutoEvolution nil=不改。
func (s *Service) UpdateMe(ctx context.Context, sess domainauth.Session, cmd UpdateMeCommand) error {
	if cmd.AutoEvolution == nil {
		return nil // 未传，不改
	}
	return s.repo.SetAutoEvolution(ctx, sess.Sub, *cmd.AutoEvolution)
}

// ChangePassword 改密：policy 校验 + argon2 verify 旧密码 + argon2 hash 新密码。
func (s *Service) ChangePassword(ctx context.Context, sess domainauth.Session, cmd ChangePasswordCommand) error {
	u, err := s.repo.FindByID(ctx, sess.Sub)
	if err != nil {
		return err
	}
	if !policyauth.CanChangePassword(u) {
		return ErrPasswordNotSet
	}
	if err := argon2.Verify(cmd.CurrentPassword, u.PasswordHash); err != nil {
		return ErrInvalidCredentials
	}
	if err := policyauth.ValidateNewPassword(cmd.CurrentPassword, cmd.NewPassword); err != nil {
		return err
	}
	newHash, err := argon2.Hash(cmd.NewPassword)
	if err != nil {
		return err
	}
	return s.repo.SetPassword(ctx, sess.Sub, newHash)
}

// SetDefaultPrompt 设置/清除默认 prompt。PromptID nil=清除，非空=设置。
func (s *Service) SetDefaultPrompt(ctx context.Context, sess domainauth.Session, cmd SetDefaultPromptCommand) error {
	promptID := ""
	if cmd.PromptID != nil {
		promptID = *cmd.PromptID
	}
	return s.repo.SetDefaultPrompt(ctx, sess.Sub, promptID)
}

// toApiUser 把 domain User 转 ApiUser（推导 auth_source）。
func toApiUser(u domainauth.User, resolver domainauth.AuthSourceResolver) ApiUser {
	return ApiUser{
		ID:              u.ID,
		Username:        u.Username,
		Role:            u.Role,
		AuthSource:      u.AuthSource(resolver),
		DefaultPromptID: u.DefaultPromptID,
		AutoEvolution:   u.AutoEvolution,
	}
}
