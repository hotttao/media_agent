// Package auth 是 auth 领域的易变业务规则（纯决策，无副作用，不查 DB）。
//
// 接收 service 加载好的事实（User/密码长度等），返回决策结果。
// 协作模式：service 加载数据 → policy.CanXxx(...) → service 落地副作用（对齐 architecture.md policy 节）。
package auth

import (
	"errors"

	domainauth "media_agent/media_auth/biz/domain/auth"
)

// CanLogin system 账号禁止登录（对齐 control-plane：role==='system' → 403）。
func CanLogin(u domainauth.User) bool {
	return u.Role != "system"
}

// CanChangePassword 仅本地密码账号（password_hash 非空）可改密（LDAP/企微派生账号不可）。
func CanChangePassword(u domainauth.User) bool {
	return u.HasPassword()
}

// 改密校验错误。
var (
	ErrPasswordTooShort = errors.New("new password must be at least 6 characters")
	ErrPasswordSame     = errors.New("new password must differ from current")
)

// ValidateNewPassword 校验新密码：len<6 → too short；new==current → same。
// currentHash 用于判断 new 是否等于 current（调用方传明文 current 与 new 比较即可）。
func ValidateNewPassword(currentPlain, newPlain string) error {
	if len(newPlain) < 6 {
		return ErrPasswordTooShort
	}
	if newPlain == currentPlain {
		return ErrPasswordSame
	}
	return nil
}
