// token_test.go：shared token 工具单测。
//
// 覆盖：OAuth HashToken（sha256 hex）、
//
//	GenerateOpaqueTokenAndHash（hex + hash 匹配）、ComputePKCEChallenge（RFC 7636 向量）、
//	GeneratePKCEVerifier / GenerateState（格式 + 唯一性）。
package token

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func TestHashToken(t *testing.T) {
	got := HashToken("abc")
	sum := sha256.Sum256([]byte("abc"))
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("HashToken mismatch: got %q, want %q", got, want)
	}
	// 相同输入确定性
	if HashToken("abc") != got {
		t.Errorf("HashToken not deterministic")
	}
}

func TestGenerateOpaqueTokenAndHash(t *testing.T) {
	plaintext, hash, err := GenerateOpaqueTokenAndHash()
	if err != nil {
		t.Fatalf("GenerateOpaqueTokenAndHash: %v", err)
	}
	// 无前缀，纯 hex
	if strings.ContainsAny(plaintext, "_") {
		t.Errorf("opaque token should have no prefix, got %q", plaintext)
	}
	if _, err := hex.DecodeString(plaintext); err != nil {
		t.Errorf("opaque token should be hex: %v", err)
	}
	if hash != HashToken(plaintext) {
		t.Errorf("hash should match HashToken(plaintext)")
	}
}

func TestComputePKCEChallenge(t *testing.T) {
	// RFC 7636 附录 B 测试向量。
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	got := ComputePKCEChallenge(verifier)
	if got != want {
		t.Errorf("PKCE challenge mismatch: got %q, want %q", got, want)
	}
	// 手工验证：base64url(sha256(verifier)) 无 padding。
	sum := sha256.Sum256([]byte(verifier))
	manual := base64.RawURLEncoding.EncodeToString(sum[:])
	if got != manual {
		t.Errorf("PKCE challenge not base64url(sha256): got %q, manual %q", got, manual)
	}
}

func TestGeneratePKCEVerifierAndState(t *testing.T) {
	v, err := GeneratePKCEVerifier()
	if err != nil {
		t.Fatalf("GeneratePKCEVerifier: %v", err)
	}
	// base64url(32 bytes) = 43 chars, no padding。
	if len(v) != 43 {
		t.Errorf("verifier length: got %d, want 43", len(v))
	}
	if strings.Contains(v, "=") {
		t.Errorf("verifier should have no padding")
	}
	s, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}
	// base64url(16 bytes) = 22 chars。
	if len(s) != 22 {
		t.Errorf("state length: got %d, want 22", len(s))
	}
	v2, _ := GeneratePKCEVerifier()
	if v == v2 {
		t.Errorf("two verifiers should differ")
	}
}
