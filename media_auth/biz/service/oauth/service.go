// Package oauth 是 oauth 用例服务层（单 domain 编排 + 1 个外部 introspect 调用）。
//
// 接受 command，编排 domain/policy/repo + session.Signer（access token）+ introspect.Client（token-exchange）
// 完成用例；禁止 import Hertz / hertz_gen / biz/dal / 其它 service 包。
// 对齐 control-plane routes/oauth-provider.ts + services/oauth-provider.ts。
package oauth

import (
	"context"
	"errors"
	"net/url"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	domainoauth "media_agent/media_auth/biz/domain/oauth"
	policyoauth "media_agent/media_auth/biz/policy/oauth"
	"media_agent/media_auth/biz/shared/client/introspect"
	"media_agent/media_auth/biz/shared/session"
	"media_agent/media_auth/biz/shared/token"
)

// Service 是 oauth 用例服务。
type Service struct {
	repo        domainoauth.OAuthRepo
	signer      *session.Signer // access token HS256 JWT（与 cookie session 同密钥）
	introspect  *introspect.Client
	refreshTTL  time.Duration // 默认 30d
	authCodeTTL time.Duration // 默认 10min
}

// NewService 构造 oauth 用例服务。
func NewService(repo domainoauth.OAuthRepo, signer *session.Signer, intro *introspect.Client, refreshTTL, authCodeTTL time.Duration) *Service {
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	if authCodeTTL <= 0 {
		authCodeTTL = 10 * time.Minute
	}
	return &Service{repo: repo, signer: signer, introspect: intro, refreshTTL: refreshTTL, authCodeTTL: authCodeTTL}
}

// OAuthError 是 OAuth2 标准错误（携带 error code + description + HTTP status）。
// handler 据此写 {error, error_description} JSON。
type OAuthError struct {
	Code        string // invalid_client / invalid_grant / ...
	Description string
	Status      int
}

func (e *OAuthError) Error() string {
	if e.Description != "" {
		return e.Code + ": " + e.Description
	}
	return e.Code
}

// 包级 OAuthError 构造器（service 内部用）。
func oauthErr(code, desc string, status int) *OAuthError {
	return &OAuthError{Code: code, Description: desc, Status: status}
}

// ── 入参 / 出参 ──────────────────────────────────────────────────────

// AuthorizeInfoResult 是 GetAuthorizeInfo 返回。
type AuthorizeInfoResult struct {
	ClientName string
	Scope      string
}

// ApproveCommand 是 ApproveAuthorization 入参。
type ApproveCommand struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
}

// ExchangeParams 是 ExchangeToken 入参（form-urlencoded 解析后）。
type ExchangeParams struct {
	GrantType        string
	ClientID         string
	Code             string
	RedirectURI      string
	CodeVerifier     string
	RefreshToken     string
	SubjectToken     string
	SubjectTokenType string
	Resource         string
	ClientSecret     string
}

// ExchangeResult 是 ExchangeToken 返回。
type ExchangeResult struct {
	AccessToken     string
	RefreshToken    string // token-exchange 不发 refresh
	TokenType       string // Bearer
	ExpiresIn       int
	IssuedTokenType string // token-exchange 专用
}

// UserInfo 是 GetUserInfo 返回。
type UserInfo struct {
	Sub      string
	Username string
	Name     string
	Email    string
}

// ── 用例 ──────────────────────────────────────────────────────────────

// GetAuthorizeInfo 返回 authorize 页所需的 client 信息（GET /api/oauth/authorize）。
func (s *Service) GetAuthorizeInfo(ctx context.Context, clientID, redirectURI, responseType, scope string) (AuthorizeInfoResult, error) {
	if responseType != "code" {
		return AuthorizeInfoResult{}, oauthErr("unsupported_response_type", "", 400)
	}
	if clientID == "" || redirectURI == "" {
		return AuthorizeInfoResult{}, oauthErr("invalid_request", "client_id and redirect_uri are required", 400)
	}
	client, err := s.repo.GetClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, domainoauth.ErrClientNotFound) {
			return AuthorizeInfoResult{}, oauthErr("invalid_client", "", 400)
		}
		return AuthorizeInfoResult{}, err
	}
	if !policyoauth.ValidateRedirectUri(client, redirectURI) {
		return AuthorizeInfoResult{}, oauthErr("invalid_redirect_uri", "", 400)
	}
	if scope == "" {
		scope = "profile"
	}
	return AuthorizeInfoResult{ClientName: client.Name, Scope: scope}, nil
}

// ApproveAuthorization 用户批准授权，生成 code 并拼 redirect_uri（POST /api/oauth/authorize）。
func (s *Service) ApproveAuthorization(ctx context.Context, userID string, cmd ApproveCommand) (string, error) {
	if cmd.ResponseType != "code" {
		return "", oauthErr("unsupported_response_type", "", 400)
	}
	if cmd.ClientID == "" || cmd.RedirectURI == "" {
		return "", oauthErr("invalid_request", "", 400)
	}
	if cmd.CodeChallenge != "" && cmd.CodeChallengeMethod != "S256" {
		return "", oauthErr("invalid_request", "only S256 code_challenge_method is supported", 400)
	}
	client, err := s.repo.GetClient(ctx, cmd.ClientID)
	if err != nil {
		if errors.Is(err, domainoauth.ErrClientNotFound) {
			return "", oauthErr("invalid_client", "", 400)
		}
		return "", err
	}
	if !policyoauth.ValidateRedirectUri(client, cmd.RedirectURI) {
		return "", oauthErr("invalid_redirect_uri", "", 400)
	}
	scope := cmd.Scope
	if scope == "" {
		scope = "profile"
	}
	// 生成 code（hex(32) 不透明串）。
	code, _, err := token.GenerateOpaqueTokenAndHash()
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateAuthCode(ctx, domainoauth.AuthCode{
		Code:          code,
		ClientID:      cmd.ClientID,
		UserID:        userID,
		RedirectURI:   cmd.RedirectURI,
		CodeChallenge: cmd.CodeChallenge,
		Scope:         scope,
		ExpiresAt:     time.Now().Add(s.authCodeTTL),
	}); err != nil {
		return "", err
	}
	// 拼 redirect_uri?code=&state=。
	u, err := url.Parse(cmd.RedirectURI)
	if err != nil {
		return "", oauthErr("invalid_request", "redirect_uri must be a URL", 400)
	}
	q := u.Query()
	q.Set("code", code)
	if cmd.State != "" {
		q.Set("state", cmd.State)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeToken 按 grant_type 分发（POST /api/oauth/token）。
func (s *Service) ExchangeToken(ctx context.Context, p ExchangeParams) (ExchangeResult, error) {
	if p.ClientID == "" {
		return ExchangeResult{}, oauthErr("invalid_request", "client_id is required", 400)
	}
	switch p.GrantType {
	case policyoauth.GrantAuthCode:
		return s.exchangeAuthCode(ctx, p)
	case policyoauth.GrantRefreshToken:
		return s.exchangeRefreshToken(ctx, p)
	case policyoauth.GrantTokenExchange:
		return s.exchangeTokenExchange(ctx, p)
	default:
		return ExchangeResult{}, oauthErr("unsupported_grant_type", "", 400)
	}
}

// exchangeAuthCode authorization_code grant。
func (s *Service) exchangeAuthCode(ctx context.Context, p ExchangeParams) (ExchangeResult, error) {
	if p.Code == "" || p.RedirectURI == "" {
		return ExchangeResult{}, oauthErr("invalid_request", "", 400)
	}
	ac, err := s.repo.ConsumeAuthCode(ctx, p.Code) // 一次性消费
	if err != nil {
		if errors.Is(err, domainoauth.ErrAuthCodeNotFound) {
			return ExchangeResult{}, oauthErr("invalid_grant", "code is invalid or expired", 400)
		}
		return ExchangeResult{}, err
	}
	if ac.ClientID != p.ClientID {
		return ExchangeResult{}, oauthErr("invalid_grant", "client_id mismatch", 400)
	}
	if ac.RedirectURI != p.RedirectURI {
		return ExchangeResult{}, oauthErr("invalid_grant", "redirect_uri mismatch", 400)
	}
	// PKCE 校验。
	if ac.HasPKCE() {
		if p.CodeVerifier == "" {
			return ExchangeResult{}, oauthErr("invalid_request", "code_verifier is required", 400)
		}
		if !policyoauth.VerifyPKCE(p.CodeVerifier, ac.CodeChallenge) {
			return ExchangeResult{}, oauthErr("invalid_grant", "PKCE verification failed", 400)
		}
	}
	u, err := s.repo.GetUserByID(ctx, ac.UserID)
	if err != nil {
		return ExchangeResult{}, errors.New("server_error: user lookup failed")
	}
	access, err := s.issueAccess(u)
	if err != nil {
		return ExchangeResult{}, err
	}
	refresh, err := s.issueRefresh(ctx, u.ID, p.ClientID)
	if err != nil {
		return ExchangeResult{}, err
	}
	return ExchangeResult{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.signer.AccessTokenTTL() / time.Second),
	}, nil
}

// exchangeRefreshToken refresh_token grant（轮转：旧删新发）。
func (s *Service) exchangeRefreshToken(ctx context.Context, p ExchangeParams) (ExchangeResult, error) {
	if p.RefreshToken == "" {
		return ExchangeResult{}, oauthErr("invalid_request", "refresh_token is required", 400)
	}
	hash := token.HashToken(p.RefreshToken)
	rt, err := s.repo.ConsumeRefreshToken(ctx, hash) // 一次性消费
	if err != nil {
		if errors.Is(err, domainoauth.ErrRefreshTokenNotFound) {
			return ExchangeResult{}, oauthErr("invalid_grant", "refresh_token is invalid or expired", 400)
		}
		return ExchangeResult{}, err
	}
	if rt.ClientID != p.ClientID {
		return ExchangeResult{}, oauthErr("invalid_grant", "client_id mismatch", 400)
	}
	u, err := s.repo.GetUserByID(ctx, rt.UserID)
	if err != nil {
		return ExchangeResult{}, errors.New("server_error: user lookup failed")
	}
	access, err := s.issueAccess(u)
	if err != nil {
		return ExchangeResult{}, err
	}
	refresh, err := s.issueRefresh(ctx, u.ID, p.ClientID)
	if err != nil {
		return ExchangeResult{}, err
	}
	return ExchangeResult{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.signer.AccessTokenTTL() / time.Second),
	}, nil
}

// exchangeTokenExchange RFC 8693 token-exchange grant。
func (s *Service) exchangeTokenExchange(ctx context.Context, p ExchangeParams) (ExchangeResult, error) {
	if p.SubjectToken == "" {
		return ExchangeResult{}, oauthErr("invalid_request", "subject_token is required", 400)
	}
	if p.SubjectTokenType != "" && p.SubjectTokenType != policyoauth.SubjectTokenTypeAccess {
		return ExchangeResult{}, oauthErr("invalid_request", "unsupported subject_token_type", 400)
	}
	if p.Resource == "" {
		return ExchangeResult{}, oauthErr("invalid_request", "resource is required (RS origin)", 400)
	}
	client, err := s.repo.GetClient(ctx, p.ClientID)
	if err != nil {
		if errors.Is(err, domainoauth.ErrClientNotFound) {
			return ExchangeResult{}, oauthErr("invalid_client", "", 400)
		}
		return ExchangeResult{}, err
	}
	// confidential client 须校验 secret（401）。
	if !policyoauth.VerifyClientSecret(client, p.ClientSecret) {
		return ExchangeResult{}, oauthErr("invalid_client", "", 401)
	}
	resourceURL, err := url.Parse(p.Resource)
	if err != nil || resourceURL.Scheme == "" || resourceURL.Host == "" {
		return ExchangeResult{}, oauthErr("invalid_request", "resource must be a URL", 400)
	}
	// 调 RS /introspect 校验 subject_token。
	userID, _, ok, err := s.introspect.VerifySubjectToken(ctx, p.SubjectToken, resourceURL.Scheme+"://"+resourceURL.Host)
	if err != nil || !ok {
		return ExchangeResult{}, oauthErr("invalid_grant", "subject_token not active", 400)
	}
	u, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return ExchangeResult{}, oauthErr("invalid_grant", "user not found", 400)
	}
	access, err := s.issueAccess(u)
	if err != nil {
		return ExchangeResult{}, err
	}
	return ExchangeResult{
		AccessToken:     access,
		TokenType:       "Bearer",
		ExpiresIn:       int(s.signer.AccessTokenTTL() / time.Second),
		IssuedTokenType: policyoauth.SubjectTokenTypeAccess,
	}, nil
}

// GetUserInfo 校验 Bearer access token 并返回用户信息（GET /api/oauth/userinfo）。
func (s *Service) GetUserInfo(ctx context.Context, accessToken string) (UserInfo, error) {
	sess, err := s.signer.Verify(accessToken)
	if err != nil {
		return UserInfo{}, oauthErr("invalid_token", "", 401)
	}
	return UserInfo{
		Sub:      sess.Sub,
		Username: sess.Username,
		Name:     sess.Name,
		Email:    sess.Email,
	}, nil
}

// CleanupExpired 删过期 auth_code + refresh_token（每小时调一次，由 main 起 goroutine）。
func (s *Service) CleanupExpired(ctx context.Context) error {
	return s.repo.CleanupExpired(ctx)
}

// ── helpers ──────────────────────────────────────────────────────────

// issueAccess 签发 access token（HS256 JWT，accessTTL）。
func (s *Service) issueAccess(u domainoauth.User) (string, error) {
	return s.signer.IssueAccessToken(domainauth.Session{
		Sub:      u.ID,
		Username: u.Username,
		Name:     u.DisplayName,
		Email:    u.Email,
		Role:     u.Role,
	})
}

// issueRefresh 生成并存储 refresh token，返回明文。
func (s *Service) issueRefresh(ctx context.Context, userID, clientID string) (string, error) {
	plaintext, hash, err := token.GenerateOpaqueTokenAndHash()
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateRefreshToken(ctx, domainoauth.RefreshToken{
		TokenHash: hash,
		ClientID:  clientID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(s.refreshTTL),
	}); err != nil {
		return "", err
	}
	return plaintext, nil
}
