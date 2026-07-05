// user.go：auth 领域的 User 实体 + 仓库接口 + auth_source 推导。
package auth

import (
	"context"
	"time"
)

// User 是账号实体（对应 media_auth.users 表的领域形态，非 GORM row）。
//
// PasswordHash 为空表示该账号无本地密码（LDAP/企微派生），不可改密。
type User struct {
	ID              string
	Username        string
	DisplayName     string
	Email           string
	PasswordHash    string // 空 = LDAP/企微派生账号
	Role            string // user | admin | system
	DefaultPromptID string
	AutoEvolution   bool
	CreatedAt       time.Time
	LastLoginAt     time.Time // 零值 = 从未登录
}

// HasPassword 是否有本地密码（决定 auth_source 与能否改密）。
func (u User) HasPassword() bool { return u.PasswordHash != "" }

// AuthSource 推导认证来源：password_hash 非空 → password；否则交由 resolver 判断 wecom/ldap。
//
// wecom 分支需查 user_identities（只读，经 AuthSourceResolver），避免 domain 直接依赖 identity domain。
// resolver 返回 "" 或 "ldap" 时回退 ldap（spec §5.1：修正 control-plane 对企微账号误报 ldap）。
func (u User) AuthSource(resolver AuthSourceResolver) string {
	if u.HasPassword() {
		return "password"
	}
	if resolver != nil {
		if src := resolver.ResolveByUserID(context.Background(), u.ID); src != "" {
			return src
		}
	}
	return "ldap"
}

// AuthSourceResolver 推导无密码账号的外部身份来源（wecom/ldap）。
//
// 由 identity domain 实现并注入（service 层装配），auth domain 不 import identity domain，
// 规避同层依赖。当前 identity domain 未实现，service 层注入返回 "" 的桩（回退 ldap）。
type AuthSourceResolver interface {
	// ResolveByUserID 查 user 的外部身份，返回 provider（如 "wecom"）或 ""。
	ResolveByUserID(ctx context.Context, userID string) string
}

// PlatformTokenEnsurer 确保 user 拥有一个 active platform service token（不存在则建）。
//
// 由 service_token domain 实现并注入（service 层装配），auth domain 不 import service_token domain。
// auth.Login 成功后 fire-and-forget 调用，为 agent 回调 control_plane 预备明文 platform token。
// 详见 spec §5.4 / ADR-0002。
type PlatformTokenEnsurer interface {
	// EnsurePlatformToken 确保 user 的 active platform token 存在；仅副作用，不返回明文。
	EnsurePlatformToken(ctx context.Context, userID string) error
}

// UserRepo 是 auth 领域的仓储接口（由 dal/repo/auth 实现）。
//
// 仓储方法收 context.Context：上层在事务内时 ctx 携带 tx（见 dal.FromContext），
// repo 据此加入事务；不在事务内时用默认连接。仓储自身不开事务。
type UserRepo interface {
	// FindByUsername 按用户名查用户（login 用）。不存在返回 (User{}, ErrUserNotFound)。
	FindByUsername(ctx context.Context, username string) (User, error)
	// FindByID 按 id 查用户（GetMe 用）。不存在返回 ErrUserNotFound。
	FindByID(ctx context.Context, id string) (User, error)
	// Upsert 创建或更新用户（LDAP 登录命中时自动建号；更新 display_name/email/last_login_at）。
	// 返回落库后的 User（含 id）。
	Upsert(ctx context.Context, username, displayName, email string) (User, error)
	// SetPassword 更新 password_hash（改密）。
	SetPassword(ctx context.Context, id, passwordHash string) error
	// SetDefaultPrompt 更新 default_prompt_id（promptID 空串=清除）。
	SetDefaultPrompt(ctx context.Context, id, promptID string) error
	// SetAutoEvolution 更新 auto_evolution。
	SetAutoEvolution(ctx context.Context, id string, autoEvolution bool) error
	// UpdateLastLogin 更新 last_login_at = now（本地登录成功后）。
	UpdateLastLogin(ctx context.Context, id string) error
}

// domain 错误。repo 实现用 errors.Is 透传，service/handler 据此映射 HTTP 状态。
var (
	ErrUserNotFound = errSentinel("user not found")
)

// errSentinel 是简单的哨兵错误类型，支持 errors.Is 比较。
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
