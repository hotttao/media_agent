// Package token 提供 service token 的生成与哈希工具（与业务无关的纯工具）。
//
// 对齐 control-plane lib/service-token.ts：明文格式 tos_<hex(32)>，DB 存 SHA-256 hex。
// 供 service_token 的 service（用户建 token）与 repo（ensure platform token）共用，避免重复。
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// GenerateTokenAndHash 生成 tos_<hex(32)> 明文 token + 其 SHA-256 hex 哈希。
//
// 明文仅返回给调用方（service 创建时一次性返回 / repo 落 platform 明文列）；
// DB 仅存 hash（非 platform）或明文（platform，见 ADR-0002）。
func GenerateTokenAndHash() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("token: read random: %w", err)
	}
	plaintext = "tos_" + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

// HashToken 对明文 token 算 SHA-256 hex（VerifyToken 把明文 hash 后查表用）。
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// GenerateOpaqueTokenAndHash 生成无前缀的 hex(32) 不透明 token + 其 SHA-256 hex 哈希。
//
// 供 oauth domain 的 authorization_code / refresh_token 用（对齐 control-plane generateRandomToken）。
// 与 service token（tos_ 前缀）区分：oauth code/refresh 是 RFC 6749 不透明串，无业务前缀。
func GenerateOpaqueTokenAndHash() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("token: read random: %w", err)
	}
	plaintext = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

// ComputePKCEChallenge 计算 PKCE S256 code_challenge = base64url(sha256(verifier))。
//
// 供 oauth domain 校验 code_verifier（对齐 control-plane computeCodeChallenge）。
func ComputePKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GeneratePKCEVerifier 生成 PKCE code_verifier = base64url(32 random bytes)（MCP OAuth / 通用 OAuth client 用）。
func GeneratePKCEVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateState 生成 OAuth state = base64url(16 random bytes)（MCP OAuth 回调防 CSRF 用）。
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("token: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
