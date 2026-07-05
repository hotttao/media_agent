// Package mcpoauth 是 mcp_oauth 领域的易变业务规则（纯决策，无副作用，不查 DB）。
//
// 刷新失败宽限判定 + permanent OAuth error 分类集中此处（对齐 control-plane shouldDropTokenOnRefreshFailure）。
package mcpoauth

import (
	"time"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
)

// permanent OAuth error codes（refresh_token/client 已死，需重新授权）。
// 其它（5xx/网络）视为暂态。
var permanentOAuthErrors = map[string]bool{
	"invalid_grant":       true,
	"invalid_client":      true,
	"unauthorized_client": true,
}

// IsPermanentOAuthError 判定 OAuth error code 是否"永久"（需重新授权）。
func IsPermanentOAuthError(oauthError string) bool {
	return permanentOAuthErrors[oauthError]
}

// ShouldDropToken 判定持续刷新失败是否应删 token + 提示重新授权。
//
// 同时满足：失败次数 >= RefreshFailMinCount 且 首次失败距今 >= RefreshFailGraceSeconds。
// firstFailAt 为 nil → false（不应发生，recordRefreshFailure 会盖章）。
// 纯函数，可单测（对齐 control-plane shouldDropTokenOnRefreshFailure）。
func ShouldDropToken(failCount int, firstFailAt *time.Time, now time.Time) bool {
	if firstFailAt == nil {
		return false
	}
	return failCount >= domainmcp.RefreshFailMinCount &&
		now.Sub(*firstFailAt) >= domainmcp.RefreshFailGraceSeconds*time.Second
}
