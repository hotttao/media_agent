// session.go：session 鉴权中间件（cookie JWT 验签 + Bearer service token + 注入 ctx + 滑动续期 + skip list）。
//
// 对齐 agent-platform browser-service/src/index.ts 的 auth 中间件：
//  1. Bearer service token（tos_xxx）→ 调 token.Service.VerifyToken → 注入 Session
//  2. Cookie JWT（httpOnly cookie `token`）→ 本地 HS256 验签 → 注入 Session + 滑动续期
//
// 归 middleware 层（基础设施）：验签独立于 auth 业务逻辑，所有 domain 的受保护路由共用。
// Session 经 ctx 向下传递，service/handler 用 SessionFromContext(ctx) 取。
package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"media_agent/media_auth/biz/domain/auth"
	tokenservice "media_agent/media_auth/biz/service/token"
	"media_agent/media_auth/biz/shared/session"
)

// sessionCtxKey 是 ctx 中携带 Session 的 key 类型。
type sessionCtxKey struct{}

// globalSigner 供 callback handler（skip list 内）手动验 cookie 取 Session。
// 由 Session(signer) 在装配时设置。
var globalSigner *session.Signer

// globalTokenSvc 供 Bearer service token 验证。由 SessionWithToken 在装配时设置。
var globalTokenSvc *tokenservice.Service

// SessionFromContext 从 ctx 取 Session（middleware 注入）；无返回 nil。
func SessionFromContext(ctx context.Context) *auth.Session {
	s, _ := ctx.Value(sessionCtxKey{}).(*auth.Session)
	return s
}

// VerifyToken 手动验 cookie/bearer token 取 Session（供 callback handler 在 skip list 内用）。
// globalSigner 未设置（InitDI 未跑）返回 error。
func VerifyToken(token string) (*auth.Session, error) {
	if globalSigner == nil {
		return nil, errors.New("session signer not initialized")
	}
	sess, err := globalSigner.Verify(token)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// skipAllPaths 是无鉴权的公开端点（所有方法都跳过；spec §3.10 + 运维端点 + internal VerifyToken）。
var skipAllPaths = map[string]bool{
	// spec §3.10 公开业务端点
	"/api/auth/login":           true,
	"/api/auth/register":        true,
	"/api/auth/wecom/enabled":   true,
	"/api/auth/wecom/authorize": true,
	"/api/auth/wecom/callback":  true,
	"/api/oauth/token":          true,
	"/api/oauth/userinfo":       true,
	// internal VerifyToken 不经 cookie/session（网络隔离/mTLS 由部署层负责）。
	"/internal/token/verify": true,
	// 运维端点（health/pprof/metrics/swagger/openapi/ping）不走 session 鉴权
	"/health":       true,
	"/ping":         true,
	"/openapi.yaml": true,
	"/metrics":      true,
}

// skipMethodPaths 是按方法跳过的端点（path → 大写 method set）。
var skipMethodPaths = map[string]map[string]bool{
	"/api/oauth/authorize": {"GET": true},
}

// skipPrefixes 是无鉴权的前缀（swagger 资源 / pprof 子路径）。
var skipPrefixes = []string{"/swagger/", "/debug/pprof/"}

// Session 返回 session 鉴权中间件（仅 cookie JWT）。
// 保留向后兼容；新代码用 SessionWithToken。
func Session(signer *session.Signer) app.HandlerFunc {
	globalSigner = signer
	return sessionMiddleware(signer, nil)
}

// SessionWithToken 返回 session 鉴权中间件（cookie JWT + Bearer service token）。
// signer 用于 cookie JWT 验签；tokenSvc 用于 Bearer service token 验证。
func SessionWithToken(signer *session.Signer, tokenSvc *tokenservice.Service) app.HandlerFunc {
	globalSigner = signer
	globalTokenSvc = tokenSvc
	return sessionMiddleware(signer, tokenSvc)
}

func sessionMiddleware(signer *session.Signer, tokenSvc *tokenservice.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		path := string(c.Request.Path())
		method := string(c.Request.Method())
		if skipAllPaths[path] {
			c.Next(ctx)
			return
		}
		if methods, ok := skipMethodPaths[path]; ok && methods[method] {
			c.Next(ctx)
			return
		}
		for _, p := range skipPrefixes {
			if strings.HasPrefix(path, p) {
				c.Next(ctx)
				return
			}
		}

		// 1. Bearer service token（tos_xxx）— 对齐 agent-platform verifyServiceToken
		if tokenSvc != nil {
			authHeader := string(c.Request.Header.Peek("Authorization"))
			if strings.HasPrefix(authHeader, "Bearer ") {
				bearer := strings.TrimPrefix(authHeader, "Bearer ")
				result, err := tokenSvc.VerifyToken(ctx, bearer)
				if err != nil || !result.Active {
					abortUnauthorized(c, "invalid service token")
					return
				}
				sess := &auth.Session{
					Sub:      result.Sub,
					Username: result.Username,
					Name:     result.Name,
					Role:     result.Role,
				}
				ctx = context.WithValue(ctx, sessionCtxKey{}, sess)
				c.Next(ctx)
				return
			}
		}

		// 2. Cookie JWT（httpOnly cookie `token`）— 本地 HS256 验签
		token := string(c.Cookie("token"))
		if token == "" {
			abortUnauthorized(c, "missing session")
			return
		}

		sess, err := signer.Verify(token)
		if err != nil {
			clearSessionCookie(c)
			abortUnauthorized(c, "invalid session")
			return
		}

		// 滑动续期：剩余 < 半周期则重签并回写 cookie。
		if signer.ShouldRenew(sess) {
			if newTok, err := signer.Renew(sess); err == nil {
				setSessionCookie(c, newTok)
			}
		}

		ctx = context.WithValue(ctx, sessionCtxKey{}, &sess)
		c.Next(ctx)
	}
}

func abortUnauthorized(c *app.RequestContext, msg string) {
	c.JSON(consts.StatusUnauthorized, map[string]string{"error": msg})
	c.Abort()
}

// setSessionCookie 下发 httpOnly cookie token（对齐 control-plane：httpOnly, sameSite=Lax, maxAge=24h, path=/）。
func setSessionCookie(c *app.RequestContext, token string) {
	c.SetCookie("token", token, 24*3600, "/", "", protocol.CookieSameSiteLaxMode, false, true)
}

// clearSessionCookie 清除 cookie token（maxAge=-1 立即过期）。
func clearSessionCookie(c *app.RequestContext) {
	c.SetCookie("token", "", -1, "/", "", protocol.CookieSameSiteLaxMode, false, true)
}
