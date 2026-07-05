// Package oauth 是 oauth 领域的易变业务规则（纯决策，无副作用，不查 DB）。
//
// redirect_uri 精确匹配、client_secret 常量时间校验、PKCE S256 校验、grant_type 校验集中此处。
package oauth

import (
	"crypto/subtle"
	"errors"

	domainoauth "media_agent/media_auth/biz/domain/oauth"
	"media_agent/media_auth/biz/shared/token"
)

// grant_type 取值。
const (
	GrantAuthCode      = "authorization_code"
	GrantRefreshToken  = "refresh_token"
	GrantTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"

	// token-exchange 期望的 subject_token_type。
	SubjectTokenTypeAccess = "urn:ietf:params:oauth:token-type:access_token"
)

// policy 错误（service 据此映射 OAuth2 error 响应）。
var (
	ErrUnsupportedResponseType = errors.New("unsupported_response_type")
	ErrInvalidRequest          = errors.New("invalid_request")
	ErrInvalidClient           = errors.New("invalid_client")
	ErrInvalidRedirectURI      = errors.New("invalid_redirect_uri")
	ErrInvalidGrant            = errors.New("invalid_grant")
	ErrUnsupportedGrantType    = errors.New("unsupported_grant_type")
	ErrPKCEFailed              = errors.New("pkce verification failed")
)

// ValidateRedirectUri 判定 redirect_uri 是否在 client 白名单内（逐元素精确匹配）。
func ValidateRedirectUri(c domainoauth.Client, redirectURI string) bool {
	for _, uri := range c.RedirectURIs {
		if uri == redirectURI {
			return true
		}
	}
	return false
}

// VerifyClientSecret 常量时间校验 client_secret（SHA-256(明文) == secret_hash）。
//
// 对齐 control-plane verifyClientSecret：用 sha256 比较避免时序泄漏。
// 公开 client（无 secret_hash）→ 永远 false。
func VerifyClientSecret(c domainoauth.Client, providedSecret string) bool {
	if c.SecretHash == "" || providedSecret == "" {
		return false
	}
	sum := token.HashToken(providedSecret)
	return subtle.ConstantTimeCompare([]byte(sum), []byte(c.SecretHash)) == 1
}

// VerifyPKCE 校验 code_verifier 对应的 S256 challenge 与 auth_code 存储的 code_challenge 一致。
//
// 仅 S256（control-plane 现状）；verifier 缺失或 challenge 不匹配 → false。
func VerifyPKCE(codeVerifier, codeChallenge string) bool {
	if codeVerifier == "" || codeChallenge == "" {
		return false
	}
	computed := token.ComputePKCEChallenge(codeVerifier)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(codeChallenge)) == 1
}
