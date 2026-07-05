// service_test.go：oauth 用例 service 单测（fake repo + real signer + httptest introspect）。
//
// 覆盖：GetAuthorizeInfo（response_type/client/redirect_uri 校验）、ApproveAuthorization（生成 code）、
//
//	ExchangeToken 三 grant 分发（auth_code 成功/PKCE 失败、refresh_token 轮转、token-exchange introspect）、
//	GetUserInfo（access token 验签）。
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	configpb "media_agent/hertz_gen/config"
	domainauth "media_agent/media_auth/biz/domain/auth"
	domainoauth "media_agent/media_auth/biz/domain/oauth"
	"media_agent/media_auth/biz/shared/client/introspect"
	"media_agent/media_auth/biz/shared/session"
	"media_agent/media_auth/biz/shared/token"
)

// ── fakes ────────────────────────────────────────────────────────────

type fakeRepo struct {
	mu      sync.Mutex
	clients map[string]domainoauth.Client
	codes   map[string]domainoauth.AuthCode     // code → authcode
	refresh map[string]domainoauth.RefreshToken // tokenHash → refresh
	users   map[string]domainoauth.User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		clients: map[string]domainoauth.Client{}, codes: map[string]domainoauth.AuthCode{},
		refresh: map[string]domainoauth.RefreshToken{}, users: map[string]domainoauth.User{},
	}
}

func (r *fakeRepo) seedClient(c domainoauth.Client) { r.clients[c.ID] = c }
func (r *fakeRepo) seedUser(u domainoauth.User)     { r.users[u.ID] = u }

func (r *fakeRepo) GetClient(_ context.Context, id string) (domainoauth.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clients[id]
	if !ok {
		return domainoauth.Client{}, domainoauth.ErrClientNotFound
	}
	return c, nil
}
func (r *fakeRepo) CreateClient(_ context.Context, c domainoauth.Client) (domainoauth.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[c.ID] = c
	return c, nil
}
func (r *fakeRepo) ListClients(_ context.Context) ([]domainoauth.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domainoauth.Client, 0, len(r.clients))
	for _, c := range r.clients {
		out = append(out, c)
	}
	return out, nil
}
func (r *fakeRepo) DeleteClient(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.clients[id]; !ok {
		return domainoauth.ErrClientNotFound
	}
	delete(r.clients, id)
	return nil
}
func (r *fakeRepo) CreateAuthCode(_ context.Context, ac domainoauth.AuthCode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codes[ac.Code] = ac
	return nil
}
func (r *fakeRepo) ConsumeAuthCode(_ context.Context, code string) (domainoauth.AuthCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ac, ok := r.codes[code]
	if !ok {
		return domainoauth.AuthCode{}, domainoauth.ErrAuthCodeNotFound
	}
	delete(r.codes, code)
	if !ac.ExpiresAt.After(time.Now()) {
		return domainoauth.AuthCode{}, domainoauth.ErrAuthCodeNotFound
	}
	return ac, nil
}
func (r *fakeRepo) CreateRefreshToken(_ context.Context, rt domainoauth.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refresh[rt.TokenHash] = rt
	return nil
}
func (r *fakeRepo) ConsumeRefreshToken(_ context.Context, tokenHash string) (domainoauth.RefreshToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.refresh[tokenHash]
	if !ok {
		return domainoauth.RefreshToken{}, domainoauth.ErrRefreshTokenNotFound
	}
	delete(r.refresh, tokenHash)
	if !rt.ExpiresAt.After(time.Now()) {
		return domainoauth.RefreshToken{}, domainoauth.ErrRefreshTokenNotFound
	}
	return rt, nil
}
func (r *fakeRepo) GetUserByID(_ context.Context, id string) (domainoauth.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return domainoauth.User{}, domainoauth.ErrUserNotFound
	}
	return u, nil
}
func (r *fakeRepo) CleanupExpired(context.Context) error { return nil }

// ── helpers ──────────────────────────────────────────────────────────

func newTestSigner(t *testing.T) *session.Signer {
	t.Helper()
	s, err := session.NewSigner(&configpb.AuthConfig{JwtSecret: "test-secret", AccessTokenExpiresInSeconds: 3600})
	if err != nil {
		t.Fatalf("newSigner: %v", err)
	}
	return s
}

// newSvc 构造 oauth service（introspect client 用默认 http）。
func newSvc(t *testing.T, repo *fakeRepo) *Service {
	t.Helper()
	return NewService(repo, newTestSigner(t), introspect.NewClient(nil), 30*24*time.Hour, 10*time.Minute)
}

// ── GetAuthorizeInfo ─────────────────────────────────────────────────

func TestGetAuthorizeInfo(t *testing.T) {
	repo := newFakeRepo()
	repo.seedClient(domainoauth.Client{ID: "c1", Name: "Client1", RedirectURIs: []string{"https://app/cb"}})
	svc := newSvc(t, repo)

	// 正常。
	info, err := svc.GetAuthorizeInfo(context.Background(), "c1", "https://app/cb", "code", "profile")
	if err != nil || info.ClientName != "Client1" || info.Scope != "profile" {
		t.Errorf("got info=%+v err=%v", info, err)
	}
	// response_type != code。
	_, err = svc.GetAuthorizeInfo(context.Background(), "c1", "https://app/cb", "token", "")
	if !oauthErrIs(err, "unsupported_response_type") {
		t.Errorf("want unsupported_response_type, got %v", err)
	}
	// client 不存在。
	_, err = svc.GetAuthorizeInfo(context.Background(), "nope", "https://app/cb", "code", "")
	if !oauthErrIs(err, "invalid_client") {
		t.Errorf("want invalid_client, got %v", err)
	}
	// redirect_uri 不匹配。
	_, err = svc.GetAuthorizeInfo(context.Background(), "c1", "https://evil/cb", "code", "")
	if !oauthErrIs(err, "invalid_redirect_uri") {
		t.Errorf("want invalid_redirect_uri, got %v", err)
	}
	// scope 缺省 → profile。
	info, _ = svc.GetAuthorizeInfo(context.Background(), "c1", "https://app/cb", "code", "")
	if info.Scope != "profile" {
		t.Errorf("default scope: want profile, got %q", info.Scope)
	}
}

// ── ApproveAuthorization ─────────────────────────────────────────────

func TestApproveAuthorization(t *testing.T) {
	repo := newFakeRepo()
	repo.seedClient(domainoauth.Client{ID: "c1", Name: "C1", RedirectURIs: []string{"https://app/cb"}})
	repo.seedUser(domainoauth.User{ID: "u1", Username: "alice", DisplayName: "Alice", Role: "user"})
	svc := newSvc(t, repo)

	redirectURI, err := svc.ApproveAuthorization(context.Background(), "u1", ApproveCommand{
		ClientID: "c1", RedirectURI: "https://app/cb", ResponseType: "code", State: "xyz", Scope: "profile",
	})
	if err != nil {
		t.Fatalf("ApproveAuthorization: %v", err)
	}
	if !strings.HasPrefix(redirectURI, "https://app/cb?code=") {
		t.Errorf("redirect_uri: %q", redirectURI)
	}
	if !strings.Contains(redirectURI, "state=xyz") {
		t.Errorf("state missing: %q", redirectURI)
	}
	// PKCE 非 S256 → invalid_request。
	_, err = svc.ApproveAuthorization(context.Background(), "u1", ApproveCommand{
		ClientID: "c1", RedirectURI: "https://app/cb", ResponseType: "code",
		CodeChallenge: "abc", CodeChallengeMethod: "plain",
	})
	if !oauthErrIs(err, "invalid_request") {
		t.Errorf("want invalid_request for non-S256, got %v", err)
	}
}

// ── ExchangeToken: authorization_code grant ──────────────────────────

func TestExchangeToken_AuthCodeGrant(t *testing.T) {
	repo := newFakeRepo()
	repo.seedClient(domainoauth.Client{ID: "c1", RedirectURIs: []string{"https://app/cb"}})
	repo.seedUser(domainoauth.User{ID: "u1", Username: "alice", DisplayName: "Alice", Role: "user"})
	svc := newSvc(t, repo)

	// 先 approve 拿 code。
	redirectURI, _ := svc.ApproveAuthorization(context.Background(), "u1", ApproveCommand{
		ClientID: "c1", RedirectURI: "https://app/cb", ResponseType: "code", Scope: "profile",
	})
	code := redirectURI[strings.Index(redirectURI, "code=")+5:]

	res, err := svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "authorization_code", ClientID: "c1", Code: code, RedirectURI: "https://app/cb",
	})
	if err != nil {
		t.Fatalf("ExchangeToken auth_code: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" || res.TokenType != "Bearer" || res.ExpiresIn != 3600 {
		t.Errorf("got res=%+v", res)
	}
	// 重复消费同一 code → invalid_grant（一次性）。
	_, err = svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "authorization_code", ClientID: "c1", Code: code, RedirectURI: "https://app/cb",
	})
	if !oauthErrIs(err, "invalid_grant") {
		t.Errorf("replay code: want invalid_grant, got %v", err)
	}
}

func TestExchangeToken_AuthCode_PKCE(t *testing.T) {
	repo := newFakeRepo()
	repo.seedClient(domainoauth.Client{ID: "c1", RedirectURIs: []string{"https://app/cb"}})
	repo.seedUser(domainoauth.User{ID: "u1", Username: "alice", DisplayName: "Alice", Role: "user"})
	svc := newSvc(t, repo)

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := token.ComputePKCEChallenge(verifier)
	redirectURI, _ := svc.ApproveAuthorization(context.Background(), "u1", ApproveCommand{
		ClientID: "c1", RedirectURI: "https://app/cb", ResponseType: "code",
		CodeChallenge: challenge, CodeChallengeMethod: "S256",
	})
	code := redirectURI[strings.Index(redirectURI, "code=")+5:]

	// 正确 verifier → 成功。
	_, err := svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "authorization_code", ClientID: "c1", Code: code, RedirectURI: "https://app/cb", CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("PKCE correct: %v", err)
	}

	// 错误 verifier → invalid_grant（PKCE failed）。重新 approve 拿新 code。
	redirectURI2, _ := svc.ApproveAuthorization(context.Background(), "u1", ApproveCommand{
		ClientID: "c1", RedirectURI: "https://app/cb", ResponseType: "code",
		CodeChallenge: challenge, CodeChallengeMethod: "S256",
	})
	code2 := redirectURI2[strings.Index(redirectURI2, "code=")+5:]
	_, err = svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "authorization_code", ClientID: "c1", Code: code2, RedirectURI: "https://app/cb", CodeVerifier: "wrong",
	})
	if !oauthErrIs(err, "invalid_grant") {
		t.Errorf("PKCE wrong: want invalid_grant, got %v", err)
	}
}

// ── ExchangeToken: refresh_token grant（轮转） ───────────────────────

func TestExchangeToken_RefreshGrant(t *testing.T) {
	repo := newFakeRepo()
	repo.seedClient(domainoauth.Client{ID: "c1", RedirectURIs: []string{"https://app/cb"}})
	repo.seedUser(domainoauth.User{ID: "u1", Username: "alice", DisplayName: "Alice", Role: "user"})
	svc := newSvc(t, repo)

	// 首次 auth_code grant 拿 refresh。
	redirectURI, _ := svc.ApproveAuthorization(context.Background(), "u1", ApproveCommand{
		ClientID: "c1", RedirectURI: "https://app/cb", ResponseType: "code",
	})
	code := redirectURI[strings.Index(redirectURI, "code=")+5:]
	first, _ := svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "authorization_code", ClientID: "c1", Code: code, RedirectURI: "https://app/cb",
	})

	// 用 refresh 换新 token。
	res, err := svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "refresh_token", ClientID: "c1", RefreshToken: first.RefreshToken,
	})
	if err != nil {
		t.Fatalf("refresh grant: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Errorf("got res=%+v", res)
	}
	// 轮转：旧 refresh 不可再用。
	_, err = svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "refresh_token", ClientID: "c1", RefreshToken: first.RefreshToken,
	})
	if !oauthErrIs(err, "invalid_grant") {
		t.Errorf("replay refresh: want invalid_grant, got %v", err)
	}
}

// ── ExchangeToken: token-exchange grant（httptest introspect） ────────

func TestExchangeToken_TokenExchangeGrant(t *testing.T) {
	// introspect httptest server：subject_token "good-token" → active+sub=u1；其它 → inactive。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("token") == "good-token" {
			json.NewEncoder(w).Encode(map[string]any{"active": true, "sub": "u1"})
		} else {
			json.NewEncoder(w).Encode(map[string]any{"active": false})
		}
	}))
	defer srv.Close()

	repo := newFakeRepo()
	secretHash := token.HashToken("s3cret")
	repo.seedClient(domainoauth.Client{ID: "c1", SecretHash: secretHash})
	repo.seedUser(domainoauth.User{ID: "u1", Username: "alice", DisplayName: "Alice", Role: "user"})
	svc := newSvc(t, repo)

	// active subject_token → 发 access token（不发 refresh）。
	res, err := svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "urn:ietf:params:oauth:grant-type:token-exchange", ClientID: "c1",
		ClientSecret: "s3cret", SubjectToken: "good-token",
		Resource:         srv.URL, // 指向 httptest origin
		SubjectTokenType: "urn:ietf:params:oauth:token-type:access_token",
	})
	if err != nil {
		t.Fatalf("token-exchange active: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken != "" {
		t.Errorf("token-exchange should issue access only, got res=%+v", res)
	}
	if res.IssuedTokenType != "urn:ietf:params:oauth:token-type:access_token" {
		t.Errorf("issued_token_type: got %q", res.IssuedTokenType)
	}

	// inactive subject_token → invalid_grant。
	_, err = svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "urn:ietf:params:oauth:grant-type:token-exchange", ClientID: "c1",
		ClientSecret: "s3cret", SubjectToken: "bad-token", Resource: srv.URL,
	})
	if !oauthErrIs(err, "invalid_grant") {
		t.Errorf("inactive subject: want invalid_grant, got %v", err)
	}

	// 错误 client_secret → invalid_client（401）。
	var oe *OAuthError
	_, err = svc.ExchangeToken(context.Background(), ExchangeParams{
		GrantType: "urn:ietf:params:oauth:grant-type:token-exchange", ClientID: "c1",
		ClientSecret: "wrong", SubjectToken: "good-token", Resource: srv.URL,
	})
	if !errors.As(err, &oe) || oe.Code != "invalid_client" || oe.Status != 401 {
		t.Errorf("wrong secret: want invalid_client 401, got %v", err)
	}
}

func TestExchangeToken_UnsupportedGrant(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.ExchangeToken(context.Background(), ExchangeParams{GrantType: "password", ClientID: "c1"})
	if !oauthErrIs(err, "unsupported_grant_type") {
		t.Errorf("want unsupported_grant_type, got %v", err)
	}
}

// ── GetUserInfo ──────────────────────────────────────────────────────

func TestGetUserInfo(t *testing.T) {
	repo := newFakeRepo()
	repo.seedUser(domainoauth.User{ID: "u1", Username: "alice", DisplayName: "Alice", Email: "a@x.com", Role: "user"})
	signer := newTestSigner(t)
	svc := NewService(repo, signer, introspect.NewClient(nil), 30*24*time.Hour, 10*time.Minute)

	// 用 signer 签一个 access token。
	access, err := signer.IssueAccessToken(domainauth.Session{
		Sub: "u1", Username: "alice", Name: "Alice", Email: "a@x.com", Role: "user",
	})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	info, err := svc.GetUserInfo(context.Background(), access)
	if err != nil || info.Sub != "u1" || info.Username != "alice" || info.Name != "Alice" {
		t.Errorf("got info=%+v err=%v", info, err)
	}
	// 无效 token → invalid_token（401）。
	_, err = svc.GetUserInfo(context.Background(), "not-a-jwt")
	if !oauthErrIs(err, "invalid_token") {
		t.Errorf("bad token: want invalid_token, got %v", err)
	}
}

// ── helpers ──────────────────────────────────────────────────────────

// oauthErrIs 判定 err 是否 *OAuthError 且 Code 匹配。
func oauthErrIs(err error, code string) bool {
	var oe *OAuthError
	if !errors.As(err, &oe) {
		return false
	}
	return oe.Code == code
}

// ── OAuth client 管理 ────────────────────────────────────────────────

func TestCreateClient_AdminOnly(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(t, repo)
	_, err := svc.CreateClient(context.Background(), "u1", "user", CreateClientCommand{
		Name: "C", RedirectURIs: []string{"https://app/cb"},
	})
	if !oauthErrIs(err, "invalid_request") {
		t.Errorf("non-admin: want invalid_request, got %v", err)
	}
}

func TestCreateClient_GeneratesSecretAndHash(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(t, repo)
	res, err := svc.CreateClient(context.Background(), "u1", "admin", CreateClientCommand{
		Name: "C", RedirectURIs: []string{"https://app/cb"},
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	// 明文 secret 非空 + tos_ 前缀。
	if res.ClientSecret == "" || !strings.HasPrefix(res.ClientSecret, "tos_") {
		t.Errorf("client_secret: got %q", res.ClientSecret)
	}
	// client id 非空。
	if res.Client.ID == "" {
		t.Errorf("client id empty")
	}
	// DB 中存的 secret_hash = sha256(明文)。
	stored := repo.clients[res.Client.ID]
	if stored.SecretHash != token.HashToken(res.ClientSecret) {
		t.Errorf("stored secret_hash mismatch: want %q, got %q",
			token.HashToken(res.ClientSecret), stored.SecretHash)
	}
	if stored.CreatedBy != "u1" {
		t.Errorf("created_by: want u1, got %q", stored.CreatedBy)
	}
}

func TestCreateClient_Validation(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	// 缺 name。
	_, err := svc.CreateClient(context.Background(), "u1", "admin", CreateClientCommand{
		RedirectURIs: []string{"https://app/cb"},
	})
	if !oauthErrIs(err, "invalid_request") {
		t.Errorf("missing name: want invalid_request, got %v", err)
	}
	// 缺 redirect_uris。
	_, err = svc.CreateClient(context.Background(), "u1", "admin", CreateClientCommand{Name: "C"})
	if !oauthErrIs(err, "invalid_request") {
		t.Errorf("missing redirect_uris: want invalid_request, got %v", err)
	}
}

func TestListClients_AdminOnly(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.ListClients(context.Background(), "user")
	if !oauthErrIs(err, "invalid_request") {
		t.Errorf("non-admin list: want invalid_request, got %v", err)
	}
}

func TestRevokeClient_AdminOnlyAndNotFound(t *testing.T) {
	repo := newFakeRepo()
	svc := newSvc(t, repo)
	// 非 admin → 403。
	err := svc.RevokeClient(context.Background(), "user", "c1")
	if !oauthErrIs(err, "invalid_request") {
		t.Errorf("non-admin revoke: want invalid_request, got %v", err)
	}
	// admin 删不存在的 → ErrClientNotFound。
	err = svc.RevokeClient(context.Background(), "admin", "nope")
	if !errors.Is(err, ErrClientNotFound) {
		t.Errorf("revoke missing: want ErrClientNotFound, got %v", err)
	}
	// admin 删存在的 → ok。
	repo.clients["c1"] = domainoauth.Client{ID: "c1", Name: "C"}
	if err := svc.RevokeClient(context.Background(), "admin", "c1"); err != nil {
		t.Errorf("revoke existing: %v", err)
	}
}
