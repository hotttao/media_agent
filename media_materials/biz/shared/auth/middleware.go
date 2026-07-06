// Package auth 给 media_materials / media_workflow 提供 OAuth client + auth 中间件。
//
// 对齐 agent-platform browser-service/src/index.ts 的 auth 模式：
//  1. Bearer service token（tos_xxx）→ 调 media_auth GET /api/auth/me 验证 → 注入 User
//  2. Cookie JWT（httpOnly cookie）→ 本地 HS256 验签（materials/workflow 自己的 jwt_secret）→ 注入 User
//
// OAuth login 流程：
//
//	GET /api/auth/login → 302 → media_auth /api/oauth/authorize（带 PKCE）
//	GET /api/auth/callback → 换 code → 取 userinfo → 签自己的 session JWT → 写 cookie
//	GET /api/auth/me → 读 cookie 返回当前用户
//	POST /api/auth/logout → 清 cookie
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/golang-jwt/jwt/v5"
)

// User 是注入 ctx 的鉴权主体（与 media_auth auth.Session 对齐）。
type User struct {
	Sub      string `json:"sub"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// Config auth 中间件配置。
type Config struct {
	JWTSecret     string // materials/workflow 共享的 JWT secret（与 media_auth 不同）
	CookieName    string // cookie 名（默认 "mw_token"）
	JWTExpiresSec int    // session JWT 有效期（默认 7 天）
	MediaAuthURL  string // media_auth base URL（OAuth + service token 验证）
	ServiceURL    string // 本服务 base URL（OAuth callback redirect_uri）
	OAuthClientID string // 在 media_auth 注册的 OAuth client_id
	SkipPaths     map[string]bool
	SkipPrefixes  []string
}

type callerKey struct{}

// CallerFromContext 从 ctx 取 User（middleware 注入）。
func CallerFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(callerKey{}).(*User)
	return u
}

// CallerID 从 ctx 取 user_id（便捷方法，无则 ""）。
func CallerID(ctx context.Context) string {
	u := CallerFromContext(ctx)
	if u == nil {
		return ""
	}
	return u.Sub
}

// —— service token 缓存 ——
type tokenCacheEntry struct {
	user      *User
	expiresAt time.Time
}
type tokenCache struct {
	mu      sync.Mutex
	entries map[string]tokenCacheEntry
	ttl     time.Duration
}

func newTokenCache(ttl time.Duration) *tokenCache {
	return &tokenCache{entries: map[string]tokenCacheEntry{}, ttl: ttl}
}

func (c *tokenCache) get(token string) (*User, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[token]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.user, true
}

func (c *tokenCache) set(token string, user *User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[token] = tokenCacheEntry{user: user, expiresAt: time.Now().Add(c.ttl)}
}

// —— PKCE 状态存储 ——
type pkceState struct {
	CodeVerifier string
	CreatedAt    time.Time
	Next         string
}
type pkceStore struct {
	mu      sync.Mutex
	pending map[string]pkceState
}

func newPKCEStore() *pkceStore {
	s := &pkceStore{pending: map[string]pkceState{}}
	go s.cleanup()
	return s
}

func (s *pkceStore) put(state string, v pkceState) {
	s.mu.Lock()
	s.pending[state] = v
	s.mu.Unlock()
}

func (s *pkceStore) take(state string) (pkceState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.pending[state]
	if ok {
		delete(s.pending, state)
	}
	return v, ok
}

func (s *pkceStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		s.mu.Lock()
		for k, v := range s.pending {
			if v.CreatedAt.Before(cutoff) {
				delete(s.pending, k)
			}
		}
		s.mu.Unlock()
	}
}

// Middleware auth 中间件实例。
type Middleware struct {
	cfg       *Config
	cache     *tokenCache
	pkce      *pkceStore
	http      *http.Client
	cookieMax int // cookie maxAge（秒）
}

func New(cfg *Config) *Middleware {
	if cfg.CookieName == "" {
		cfg.CookieName = "mw_token"
	}
	if cfg.JWTExpiresSec <= 0 {
		cfg.JWTExpiresSec = 7 * 24 * 3600
	}
	return &Middleware{
		cfg:       cfg,
		cache:     newTokenCache(60 * time.Second),
		pkce:      newPKCEStore(),
		http:      &http.Client{Timeout: 10 * time.Second},
		cookieMax: cfg.JWTExpiresSec,
	}
}

// Handler 返回 Hertz 中间件。
func (m *Middleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		path := string(c.Request.Path())
		if m.cfg.SkipPaths[path] {
			c.Next(ctx)
			return
		}
		for _, p := range m.cfg.SkipPrefixes {
			if strings.HasPrefix(path, p) {
				c.Next(ctx)
				return
			}
		}

		// 1. Bearer service token → 调 media_auth /api/auth/me
		authHeader := string(c.Request.Header.Peek("Authorization"))
		if strings.HasPrefix(authHeader, "Bearer ") {
			bearer := strings.TrimPrefix(authHeader, "Bearer ")
			user, err := m.verifyServiceToken(ctx, bearer)
			if err != nil || user == nil {
				c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid service token"})
				c.Abort()
				return
			}
			ctx = context.WithValue(ctx, callerKey{}, user)
			c.Next(ctx)
			return
		}

		// 2. Cookie JWT → 本地验签
		token := string(c.Cookie(m.cfg.CookieName))
		if token == "" {
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "missing session"})
			c.Abort()
			return
		}
		user, err := m.verifyJWT(token)
		if err != nil {
			m.clearCookie(c)
			c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid session"})
			c.Abort()
			return
		}
		ctx = context.WithValue(ctx, callerKey{}, user)
		c.Next(ctx)
	}
}

// verifyServiceToken 调 media_auth GET /api/auth/me with Bearer token。
func (m *Middleware) verifyServiceToken(ctx context.Context, token string) (*User, error) {
	if cached, ok := m.cache.get(token); ok {
		return cached, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.MediaAuthURL+"/api/auth/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("verify service token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media_auth /api/auth/me: status %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var me struct {
		ID       string `json:"id"`
		Sub      string `json:"sub"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return nil, fmt.Errorf("decode /api/auth/me: %w", err)
	}
	sub := me.Sub
	if sub == "" {
		sub = me.ID
	}
	user := &User{Sub: sub, Username: me.Username, Name: me.Name, Role: me.Role}
	m.cache.set(token, user)
	return user, nil
}

// verifyJWT 本地 HS256 验签。
func (m *Middleware) verifyJWT(token string) (*User, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(m.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	return &User{
		Sub:      getString(claims, "sub"),
		Username: getString(claims, "username"),
		Name:     getString(claims, "name"),
		Role:     getString(claims, "role"),
	}, nil
}

// signJWT 用本地 JWT_SECRET 签发 session token。
func (m *Middleware) signJWT(user *User) (string, error) {
	claims := jwt.MapClaims{
		"sub":      user.Sub,
		"username": user.Username,
		"name":     user.Name,
		"role":     user.Role,
		"exp":      time.Now().Add(time.Duration(m.cfg.JWTExpiresSec) * time.Second).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.cfg.JWTSecret))
}

func (m *Middleware) setCookie(c *app.RequestContext, token string) {
	c.SetCookie(m.cfg.CookieName, token, m.cookieMax, "/", "", protocol.CookieSameSiteLaxMode, false, true)
}

func (m *Middleware) clearCookie(c *app.RequestContext) {
	c.SetCookie(m.cfg.CookieName, "", -1, "/", "", protocol.CookieSameSiteLaxMode, false, true)
}

// —— OAuth login/callback handlers ——

// LoginHandler GET /api/auth/login → 302 → media_auth /api/oauth/authorize（带 PKCE）
func (m *Middleware) LoginHandler(ctx context.Context, c *app.RequestContext) {
	verifier := generateRandomString(32)
	challenge := generateCodeChallenge(verifier)
	state := generateRandomString(16)
	next := string(c.Query("next"))
	if next != "" && (!strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//")) {
		next = ""
	}
	m.pkce.put(state, pkceState{CodeVerifier: verifier, CreatedAt: time.Now(), Next: next})

	params := url.Values{
		"client_id":             {m.cfg.OAuthClientID},
		"redirect_uri":          {m.cfg.ServiceURL + "/api/auth/callback"},
		"response_type":         {"code"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	c.Redirect(consts.StatusFound, []byte(m.cfg.MediaAuthURL+"/api/oauth/authorize?"+params.Encode()))
}

// CallbackHandler GET /api/auth/callback → 换 code → 取 userinfo → 签 JWT → 写 cookie → 302 → next
func (m *Middleware) CallbackHandler(ctx context.Context, c *app.RequestContext) {
	code := string(c.Query("code"))
	state := string(c.Query("state"))
	if code == "" || state == "" {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "missing code or state"})
		return
	}
	pkce, ok := m.pkce.take(state)
	if !ok {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "invalid or expired state"})
		return
	}
	// 1. 换 access_token
	tokenResp, err := m.exchangeCode(ctx, code, pkce.CodeVerifier)
	if err != nil {
		c.JSON(consts.StatusBadGateway, map[string]string{"error": "token exchange failed: " + err.Error()})
		return
	}
	// 2. 取 userinfo
	userinfo, err := m.fetchUserinfo(ctx, tokenResp.AccessToken)
	if err != nil {
		c.JSON(consts.StatusBadGateway, map[string]string{"error": "userinfo failed: " + err.Error()})
		return
	}
	// 3. 签自己的 session JWT
	user := &User{Sub: userinfo.Sub, Username: userinfo.Username, Name: userinfo.Name, Role: "user"}
	jwtToken, err := m.signJWT(user)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": "sign session failed"})
		return
	}
	// 4. 写 cookie + 302 → next
	m.setCookie(c, jwtToken)
	redirect := pkce.Next
	if redirect == "" {
		redirect = "/"
	}
	c.Redirect(consts.StatusFound, []byte(redirect))
}

// MeHandler GET /api/auth/me → 读 cookie 返回当前用户
func (m *Middleware) MeHandler(ctx context.Context, c *app.RequestContext) {
	user := CallerFromContext(ctx)
	if user == nil {
		c.JSON(consts.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	c.JSON(consts.StatusOK, map[string]string{
		"id":       user.Sub,
		"sub":      user.Sub,
		"username": user.Username,
		"name":     user.Name,
		"role":     user.Role,
	})
}

// LogoutHandler POST /api/auth/logout → 清 cookie
func (m *Middleware) LogoutHandler(ctx context.Context, c *app.RequestContext) {
	m.clearCookie(c)
	c.JSON(consts.StatusOK, map[string]bool{"success": true})
}

// —— OAuth helpers ——

type oauthTokenResp struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type oauthUserinfo struct {
	Sub      string `json:"sub"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

func (m *Middleware) exchangeCode(ctx context.Context, code, verifier string) (*oauthTokenResp, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {m.cfg.ServiceURL + "/api/auth/callback"},
		"client_id":     {m.cfg.OAuthClientID},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.MediaAuthURL+"/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var out oauthTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (m *Middleware) fetchUserinfo(ctx context.Context, accessToken string) (*oauthUserinfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.cfg.MediaAuthURL+"/api/oauth/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo status %d", resp.StatusCode)
	}
	var out oauthUserinfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// —— PKCE helpers ——

func generateRandomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	return base64URLEncode(b)
}

func generateCodeChallenge(verifier string) string {
	h := newSHA256([]byte(verifier))
	return base64URLEncode(h)
}

// —— utils ——

func getString(claims jwt.MapClaims, key string) string {
	v, ok := claims[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

var _ = errors.New // suppress unused import
