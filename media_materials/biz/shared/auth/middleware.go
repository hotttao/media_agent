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
	"bytes"
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
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/delegationaudit"
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
	JWTSecret              string // materials/workflow 共享的 JWT secret（与 media_auth 不同）
	CookieName             string // cookie 名（默认 "mw_token"）
	JWTExpiresSec          int    // session JWT 有效期（默认 7 天）
	MediaAuthURL           string // media_auth base URL（OAuth + service token 验证）
	ServiceURL             string // 本服务 base URL（OAuth callback redirect_uri）
	OAuthClientID          string // 在 media_auth 注册的 OAuth client_id
	OAuthClientSecret      string // OAuth client_secret（机密 client 必填）
	KratosPublicURL        string // Ory Kratos public URL；配置后浏览器会话优先使用 Kratos
	MediaAuthInternalToken string // internal identity/team lookup credential
	InternalJWTVerifier    *internaljwt.Verifier
	AgentJWTVerifier       *internaljwt.Verifier
	// DevUserID enables a local-only authenticated session when explicitly set.
	// It is intentionally empty by default and must never be configured in prod.
	DevUserID         string
	DevOrganizationID string
	SkipPaths         map[string]bool
	SkipPrefixes      []string
}

type callerKey struct{}
type teamsKey struct{}
type organizationsKey struct{}

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

// TeamsFromContext 从 ctx 取 caller 的 team ID 列表（middleware 注入，可能为 nil）。
func TeamsFromContext(ctx context.Context) []string {
	t, _ := ctx.Value(teamsKey{}).([]string)
	return t
}

func OrganizationsFromContext(ctx context.Context) []string {
	ids, _ := ctx.Value(organizationsKey{}).([]string)
	return ids
}

func HasOrganization(ctx context.Context, organizationID string) bool {
	if organizationID == "" {
		return false
	}
	for _, id := range OrganizationsFromContext(ctx) {
		if id == organizationID {
			return true
		}
	}
	return false
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

// —— team 缓存 ——
type teamCacheEntry struct {
	teams     []string
	expiresAt time.Time
}
type teamCache struct {
	mu      sync.Mutex
	entries map[string]teamCacheEntry
	ttl     time.Duration
}

func newTeamCache(ttl time.Duration) *teamCache {
	return &teamCache{entries: map[string]teamCacheEntry{}, ttl: ttl}
}

func (c *teamCache) get(userID string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[userID]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.teams, true
}

func (c *teamCache) set(userID string, teams []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[userID] = teamCacheEntry{teams: teams, expiresAt: time.Now().Add(c.ttl)}
}

// Middleware auth 中间件实例。
type Middleware struct {
	cfg           *Config
	cache         *tokenCache
	teams         *teamCache
	organizations *teamCache
	pkce          *pkceStore
	http          *http.Client
	cookieMax     int
}

func New(cfg *Config) *Middleware {
	if cfg.CookieName == "" {
		cfg.CookieName = "mw_token"
	}
	if cfg.JWTExpiresSec <= 0 {
		cfg.JWTExpiresSec = 7 * 24 * 3600
	}
	return &Middleware{
		cfg:           cfg,
		cache:         newTokenCache(60 * time.Second),
		teams:         newTeamCache(60 * time.Second),
		organizations: newTeamCache(60 * time.Second),
		pkce:          newPKCEStore(),
		http:          &http.Client{Timeout: 10 * time.Second},
		cookieMax:     cfg.JWTExpiresSec,
	}
}

// Handler 返回 Hertz 中间件。
func (m *Middleware) Handler() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		path := string(c.Request.Path())
		if m.cfg.DevUserID != "" && path != "/api/auth/login" && path != "/api/auth/callback" && path != "/api/auth/logout" {
			ctx = context.WithValue(ctx, callerKey{}, &User{Sub: m.cfg.DevUserID, Username: m.cfg.DevUserID, Name: "Local Developer", Role: "admin"})
			if m.cfg.DevOrganizationID != "" {
				ctx = context.WithValue(ctx, organizationsKey{}, []string{m.cfg.DevOrganizationID})
			}
			c.Next(ctx)
			return
		}
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
			if m.cfg.AgentJWTVerifier != nil {
				if claims, err := m.cfg.AgentJWTVerifier.Verify(ctx, bearer); err == nil {
					principal, err := internaljwt.ValidateAgentRequest(claims, internaljwt.AgentRequest{Method: string(c.Request.Header.Method()), URI: string(c.Request.URI().RequestURI()), ContentType: string(c.Request.Header.ContentType()), Body: c.Request.Body()})
					if err != nil {
						c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid Agent request binding"})
						c.Abort()
						return
					}
					defer delegationaudit.BusinessOutcome(ctx, principal, "materials", string(c.Request.Header.Method()), c.FullPath(), c.Response.StatusCode)()
					organizationIDs, err := m.fetchOrganizationsLive(ctx, principal.UserID)
					if err != nil {
						c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "organization authorization unavailable"})
						c.Abort()
						return
					}
					member := false
					for _, id := range organizationIDs {
						if id == principal.OrganizationID {
							member = true
							break
						}
					}
					if !member {
						c.JSON(consts.StatusForbidden, map[string]string{"error": "Agent organization access denied"})
						c.Abort()
						return
					}
					ctx = internaljwt.ContextWithAgentPrincipal(ctx, principal)
					ctx = context.WithValue(ctx, callerKey{}, &User{Sub: principal.UserID, Username: principal.UserID, Name: principal.UserID, Role: "user"})
					ctx = context.WithValue(ctx, organizationsKey{}, []string{principal.OrganizationID})
					if teams, _ := m.fetchTeams(ctx, principal.UserID); teams != nil {
						ctx = context.WithValue(ctx, teamsKey{}, teams)
					}
					c.Next(ctx)
					return
				}
			}
			if m.cfg.InternalJWTVerifier != nil {
				if claims, verifyErr := m.cfg.InternalJWTVerifier.Verify(ctx, bearer); verifyErr == nil {
					if claims.AuthSource == "api_token" {
						userID, ok := strings.CutPrefix(claims.Subject, "User:")
						if !ok || userID == "" || (claims.Actor != nil && claims.Actor.Sub != "" && claims.Actor.Sub != claims.Subject) {
							c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid API Token principal"})
							c.Abort()
							return
						}
						role := claims.Role
						if role == "" {
							role = "user"
						}
						user := &User{Sub: userID, Username: userID, Name: userID, Role: role}
						ctx = context.WithValue(ctx, callerKey{}, user)
						teamIDs, _ := m.fetchTeams(ctx, user.Sub)
						if teamIDs != nil {
							ctx = context.WithValue(ctx, teamsKey{}, teamIDs)
						}
						organizationIDs := []string(nil)
						if claims.OrganizationID != "" {
							organizationIDs = []string{claims.OrganizationID}
						} else {
							organizationIDs, verifyErr = m.fetchOrganizations(ctx, user.Sub)
							if verifyErr != nil {
								c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "organization authorization unavailable"})
								c.Abort()
								return
							}
						}
						ctx = context.WithValue(ctx, organizationsKey{}, organizationIDs)
						c.Next(ctx)
						return
					}
					user, resolveErr := m.verifyKratosSessionForIdentity(ctx, string(c.Request.Header.Peek("Cookie")), claims.Subject)
					if resolveErr != nil {
						c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid internal identity"})
						c.Abort()
						return
					}
					ctx = context.WithValue(ctx, callerKey{}, user)
					teamIDs, _ := m.fetchTeams(ctx, user.Sub)
					if teamIDs != nil {
						ctx = context.WithValue(ctx, teamsKey{}, teamIDs)
					}
					organizationIDs, orgErr := m.fetchOrganizations(ctx, user.Sub)
					if orgErr != nil {
						c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "organization authorization unavailable"})
						c.Abort()
						return
					}
					ctx = context.WithValue(ctx, organizationsKey{}, organizationIDs)
					c.Next(ctx)
					return
				}
			}
			user, err := m.verifyServiceToken(ctx, bearer)
			if err != nil || user == nil {
				c.JSON(consts.StatusUnauthorized, map[string]string{"error": "invalid service token"})
				c.Abort()
				return
			}
			ctx = context.WithValue(ctx, callerKey{}, user)
			teamIDs, _ := m.fetchTeams(ctx, user.Sub)
			if teamIDs != nil {
				ctx = context.WithValue(ctx, teamsKey{}, teamIDs)
			}
			organizationIDs, orgErr := m.fetchOrganizations(ctx, user.Sub)
			if orgErr != nil {
				c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "organization authorization unavailable"})
				c.Abort()
				return
			}
			ctx = context.WithValue(ctx, organizationsKey{}, organizationIDs)
			c.Next(ctx)
			return
		}

		// 2. Cookie JWT → 本地验签
		token := string(c.Cookie(m.cfg.CookieName))
		if token == "" {
			if user, err := m.verifyKratosSession(ctx, string(c.Request.Header.Peek("Cookie"))); err == nil && user != nil {
				ctx = context.WithValue(ctx, callerKey{}, user)
				teamIDs, _ := m.fetchTeams(ctx, user.Sub)
				if teamIDs != nil {
					ctx = context.WithValue(ctx, teamsKey{}, teamIDs)
				}
				organizationIDs, orgErr := m.fetchOrganizations(ctx, user.Sub)
				if orgErr != nil {
					c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "organization authorization unavailable"})
					c.Abort()
					return
				}
				ctx = context.WithValue(ctx, organizationsKey{}, organizationIDs)
				c.Next(ctx)
				return
			}
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
		// 同步取 teams（cookie 路径是浏览器用户，team 校验常用）
		teamIDs, _ := m.fetchTeams(ctx, user.Sub)
		if teamIDs != nil {
			ctx = context.WithValue(ctx, teamsKey{}, teamIDs)
		}
		organizationIDs, orgErr := m.fetchOrganizations(ctx, user.Sub)
		if orgErr != nil {
			c.JSON(consts.StatusServiceUnavailable, map[string]string{"error": "organization authorization unavailable"})
			c.Abort()
			return
		}
		ctx = context.WithValue(ctx, organizationsKey{}, organizationIDs)
		c.Next(ctx)
	}
}

// verifyKratosSession asks Kratos to validate the browser's httpOnly session
// cookie. Identity traits remain profile data; authorization stays in Keto and
// the owning business service.
func (m *Middleware) verifyKratosSession(ctx context.Context, cookie string) (*User, error) {
	return m.verifyKratosSessionForIdentity(ctx, cookie, "")
}

func (m *Middleware) verifyKratosSessionForIdentity(ctx context.Context, cookie, expectedIdentityID string) (*User, error) {
	if m.cfg.KratosPublicURL == "" || cookie == "" {
		return nil, errors.New("kratos session unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(m.cfg.KratosPublicURL, "/")+"/sessions/whoami", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookie)
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("verify kratos session: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kratos whoami: status %d", resp.StatusCode)
	}
	var session struct {
		Identity struct {
			ID     string `json:"id"`
			Traits struct {
				Email string `json:"email"`
				Name  struct {
					First string `json:"first"`
					Last  string `json:"last"`
				} `json:"name"`
			} `json:"traits"`
		} `json:"identity"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("decode kratos whoami: %w", err)
	}
	if session.Identity.ID == "" {
		return nil, errors.New("kratos identity id is empty")
	}
	if expectedIdentityID != "" && session.Identity.ID != expectedIdentityID {
		return nil, errors.New("internal JWT subject does not match Kratos session")
	}
	name := strings.TrimSpace(session.Identity.Traits.Name.First + " " + session.Identity.Traits.Name.Last)
	if name == "" {
		name = session.Identity.Traits.Email
	}
	return m.resolveKratosIdentity(ctx, session.Identity.ID, session.Identity.Traits.Email, name)
}

func (m *Middleware) resolveKratosIdentity(ctx context.Context, identityID, email, name string) (*User, error) {
	body, err := json.Marshal(map[string]string{"identity_id": identityID, "email": email, "name": name})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(m.cfg.MediaAuthURL, "/")+"/internal/identities/kratos:resolve", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.cfg.MediaAuthInternalToken != "" {
		req.Header.Set("X-Media-Auth-Internal-Token", m.cfg.MediaAuthInternalToken)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resolve Kratos identity: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resolve Kratos identity: status %d", resp.StatusCode)
	}
	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode resolved identity: %w", err)
	}
	if user.Sub == "" {
		return nil, errors.New("resolved business user id is empty")
	}
	return &user, nil
}

// prefetchTeams 异步预热 team 缓存（service token 路径用）。
func (m *Middleware) prefetchTeams(userID string) {
	if cached, ok := m.teams.get(userID); ok {
		_ = cached
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.fetchTeams(ctx, userID)
}

// fetchTeams 调 media_auth POST /internal/teams:byUser 取 caller 的 team ID 列表（带缓存）。
// 供 team 可见性校验（material/generated_asset/virtual_ip 的 visibility=team 分支）。
// 失败时返回 nil 不阻塞请求（team 校验降级为 false，不影响 private/public）。
func (m *Middleware) fetchTeams(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	if cached, ok := m.teams.get(userID); ok {
		return cached, nil
	}
	body, err := json.Marshal(map[string]string{"user_id": userID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.MediaAuthURL+"/internal/teams:byUser", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.cfg.MediaAuthInternalToken != "" {
		req.Header.Set("X-Media-Auth-Internal-Token", m.cfg.MediaAuthInternalToken)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch teams: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media_auth /internal/teams:byUser: status %d", resp.StatusCode)
	}
	var out struct {
		TeamIDs []string `json:"team_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode teams resp: %w", err)
	}
	m.teams.set(userID, out.TeamIDs)
	return out.TeamIDs, nil
}

func (m *Middleware) fetchOrganizations(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	if cached, ok := m.organizations.get(userID); ok {
		return cached, nil
	}
	return m.fetchOrganizationsLive(ctx, userID)
}

// Agent delegation must not reuse a membership cache after access is removed.
func (m *Middleware) fetchOrganizationsLive(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	body, err := json.Marshal(map[string]string{"user_id": userID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.cfg.MediaAuthURL+"/internal/organizations:byUser", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.cfg.MediaAuthInternalToken != "" {
		req.Header.Set("X-Media-Auth-Internal-Token", m.cfg.MediaAuthInternalToken)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch organizations: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media_auth /internal/organizations:byUser: status %d", resp.StatusCode)
	}
	var out struct {
		OrganizationIDs []string `json:"organization_ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode organizations resp: %w", err)
	}
	m.organizations.set(userID, out.OrganizationIDs)
	return out.OrganizationIDs, nil
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
	if m.cfg.KratosPublicURL != "" {
		next := string(c.Query("next"))
		if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
			next = "/"
		}
		returnTo := strings.TrimRight(m.cfg.ServiceURL, "/") + next
		loginURL := strings.TrimRight(m.cfg.KratosPublicURL, "/") + "/self-service/login/browser?return_to=" + url.QueryEscape(returnTo)
		c.Redirect(consts.StatusFound, []byte(loginURL))
		return
	}
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
		"client_secret": {m.cfg.OAuthClientSecret},
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
