// Package token provides OAuth opaque secrets, hashing, PKCE and CSRF state.
// It has no retired ServiceToken generator or business Bearer verification.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// HashToken hashes OAuth client/refresh credentials, not ordinary API Bearers.
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// GenerateOpaqueTokenAndHash 生成无前缀的 hex(32) 不透明 token + 其 SHA-256 hex 哈希。
//
// 供 OAuth authorization_code / refresh_token / Client Secret 使用，无业务前缀。
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
