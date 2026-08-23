// Package auth 存放 auth（认证/会话）领域的稳定业务核心：Session 值对象、User 实体、
// 仓库接口、SessionSigner 接口、不变量。
//
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM / JWT SDK。
// SessionSigner 是接口，JWT 实现归 service 层（依赖 config 的 JWT_SECRET）。
package auth

import "time"

// Session 是一次已认证会话的值对象（JWT payload 的领域映射）。
//
// 与 control-plane JwtPayload 对齐：sub=db user id, username, name(display_name), email, role, exp。
// 不可变；续期由 SessionSigner.Renew 产出新 token，不改本对象。
type Session struct {
	Sub         string    // db user id
	Username    string    // 登录账号名
	Name        string    // 展示名（display_name）
	Email       string    // 可空
	Role        string    // user | admin | system
	AuthVersion int64     // 凭据版本；用户撤销访问时递增
	Exp         time.Time // 过期时间
}

// SessionSigner 是 session JWT 的签发/验签接口（HS256）。
//
// 实现归 service 层（依赖 config.AuthConfig.JwtSecret）；domain 只依赖接口，
// 便于单测注入 fake signer。与 control-plane generateToken/verifyToken/shouldRenewToken/renewToken 对齐。
type SessionSigner interface {
	// Sign 签发 session 为 JWT 字符串。
	Sign(s Session) (string, error)
	// Verify 验签 token 并还原 Session；失败返回 error。
	Verify(token string) (Session, error)
	// ShouldRenew 剩余时间 < 半周期则需续期（滑动续期）。
	ShouldRenew(s Session) bool
	// Renew 用原 session 重新签发新 exp 的 token（滑动续期）。
	Renew(s Session) (string, error)
}
