// session.go：session 鉴权中间件（cookie JWT 验签 + 注入 ctx + 滑动续期 + skip list）。
//
// 对齐 control-plane src/index.ts 的认证中间件（cookie 段）：读 cookie `token` →
// SessionSigner.Verify → 注入 Session 到 context.Context → 滑动续期（ShouldRenew→Renew 回写 cookie）。
// spec §3.8 skip list 短路（login/oauth-token 等公开端点不验）。
//
// 归 middleware 层（基础设施）：验签独立于 auth 业务逻辑，所有 domain 的受保护路由共用。
// Session 经 ctx 向下传递，service/handler 用 FromContext(ctx) 取。
package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"media_agent/media_auth/biz/domain/auth"
	"media_agent/media_auth/biz/shared/session"
)

// sessionCtxKey 是 ctx 中携带 Session 的 key 类型。
type sessionCtxKey struct{}

// globalSigner 供 callback handler（skip list 内）手动验 cookie 取 Session。
// 由 Session(signer) 在装配时设置。
var globalSigner *session.Signer

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

// skipAllPaths 是无 JWT 鉴权的公开端点（所有方法都跳过；spec §3.8 + 运维端点 + internal mTLS）。
var skipAllPaths = map[string]bool{
	// spec §3.8 公开业务端点
	"/api/auth/login":           true,
	"/api/auth/wecom/enabled":   true,
	"/api/auth/wecom/authorize": true,
	"/api/auth/wecom/callback":  true,
	"/api/oauth/token":          true,
	"/api/oauth/userinfo":       true,
	// internal VerifyToken 走 mTLS 通道，不经 cookie/session 中间件（spec §3.9）。
	"/internal/token/verify": true,
	// 运维端点（health/pprof/metrics/swagger/openapi/ping）不走 session 鉴权
	"/health":       true,
	"/ping":         true,
	"/openapi.yaml": true,
	"/metrics":      true,
}

// skipMethodPaths 是按方法跳过的端点（path → 大写 method set）。
//
// /api/oauth/authorize：GET 公开（authorize 页取 client 信息），POST 需 session（用户批准）。
// control-plane 现状：GET /authorize 无鉴权，POST /authorize 用 c.get('user')。
var skipMethodPaths = map[string]map[string]bool{
	"/api/oauth/authorize": {"GET": true},
}

// skipPrefixes 是无鉴权的前缀（swagger 资源 / pprof 子路径）。
var skipPrefixes = []string{"/swagger/", "/debug/pprof/"}

// Session 返回 session 鉴权中间件。
//
// 流程：skip list 短路 → 读 cookie token → Verify → 注入 ctx → 滑动续期回写 cookie。
// 无 cookie / 验签失败：API 请求 401；后续可扩展页面 302 → /login（media_auth 仅 API，统一 401）。
func Session(signer *session.Signer) app.HandlerFunc {
	globalSigner = signer
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

		token := string(c.Cookie("token"))
		if token == "" {
			abortUnauthorized(c, "missing session")
			return
		}

		sess, err := signer.Verify(token)
		if err != nil {
			// 验签失败：清 cookie + 401。
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

		// 注入 Session 到 ctx，供下游 service/handler 取。
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
