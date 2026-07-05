// Package team 是 team 领域的易变业务规则（纯决策，无副作用，不查 DB）。
//
// 末位 admin 保护、邀请过期判定等规则集中此处；service 加载事实后调 policy 决策。
package team

import (
	"errors"
	"time"

	domainteam "media_agent/media_auth/biz/domain/team"
)

// RoleAdmin / RoleMember 与 domain 一致，便于 policy 内部引用。
const (
	RoleAdmin  = string(domainteam.RoleAdmin)
	RoleMember = string(domainteam.RoleMember)
)

// 末位 admin 保护错误（service 据此映射 409）。
var (
	ErrCannotDemoteLastAdmin = errors.New("cannot demote the last admin")
	ErrCannotRemoveLastAdmin = errors.New("cannot remove the last admin; delete the team instead")
	ErrInvalidRole           = errors.New("invalid role: must be admin or member")
)

// IsValidRole 校验 role 取值合法。
func IsValidRole(role string) bool {
	return role == RoleAdmin || role == RoleMember
}

// CanDemoteMember 判定能否把目标成员降级（非 admin）。
//
// 末位 admin 保护：目标当前是 admin 且 team 内 admin 仅 1 名 → 拒绝降级。
// canDemote=false 时返回 ErrCannotDemoteLastAdmin；true 返回 nil。
// targetRole==admin 时无需校验（升 admin 不触发保护）。
func CanDemoteMember(targetCurrentRole string, targetNewRole string, adminCount int) error {
	if targetNewRole == RoleAdmin {
		return nil // 升 admin，不影响末位保护
	}
	if targetCurrentRole == RoleAdmin && adminCount <= 1 {
		return ErrCannotDemoteLastAdmin
	}
	return nil
}

// CanRemoveMember 判定能否移除目标成员。
//
// 末位 admin 保护：目标当前是 admin 且 team 内 admin 仅 1 名 → 拒绝移除。
// canRemove=false 时返回 ErrCannotRemoveLastAdmin；true 返回 nil。
func CanRemoveMember(targetCurrentRole string, adminCount int) error {
	if targetCurrentRole == RoleAdmin && adminCount <= 1 {
		return ErrCannotRemoveLastAdmin
	}
	return nil
}

// IsExpired 判定邀请是否过期。ExpiresAt 为 nil（不过期）→ false。
func IsExpired(expiresAt *time.Time, now time.Time) bool {
	if expiresAt == nil {
		return false
	}
	return !expiresAt.After(now) // expires_at <= now 即过期
}

// DefaultInviteExpiry 默认邀请有效期 7 天（CreateInvite 未传 expires_in_days 时）。
const DefaultInviteExpiry = 7 * 24 * time.Hour
