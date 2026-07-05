// Package servicetoken 是 service_token 领域的易变业务规则（纯决策，无副作用，不查 DB）。
package servicetoken

import (
	"errors"

	domainst "media_agent/media_auth/biz/domain/service_token"
)

// 撤销权限错误。
var (
	ErrCannotRevokePlatform = errors.New("platform tokens cannot be revoked")
)

// CanRevoke 判定 user 能否撤销该 token：仅本人非 platform token 可撤销。
//
// platform token 由系统管理，用户不可撤销（control-plane 现状：revoke WHERE is_platform=false）。
func CanRevoke(t domainst.Token, userID string) error {
	if t.IsPlatform {
		return ErrCannotRevokePlatform
	}
	if t.CreatedBy != userID {
		return domainst.ErrNotFound // 不属本人，统一按 not found 返回（不暴露存在性）
	}
	return nil
}
