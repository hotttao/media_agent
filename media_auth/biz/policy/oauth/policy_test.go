// policy_test.go：oauth 领域纯决策规则单测（不查 DB）。
//
// 覆盖：redirect_uri 精确匹配、client_secret 常量时间校验、PKCE S256 校验。
package oauth

import (
	"testing"

	domainoauth "media_agent/media_auth/biz/domain/oauth"
	"media_agent/media_auth/biz/shared/token"
)

func TestValidateRedirectUri(t *testing.T) {
	client := domainoauth.Client{
		ID:           "c1",
		RedirectURIs: []string{"https://app.example.com/callback", "http://localhost:3000/cb"},
	}
	cases := []struct {
		name        string
		redirectURI string
		want        bool
	}{
		{"精确匹配（含端口/路径）", "http://localhost:3000/cb", true},
		{"匹配第二个", "https://app.example.com/callback", true},
		{"前缀匹配不算（必须精确）", "https://app.example.com/callback/extra", false},
		{"未注册的 URI", "https://evil.com/callback", false},
		{"空串", "", false},
		{"trailing slash 差异", "https://app.example.com/callback/", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidateRedirectUri(client, c.redirectURI); got != c.want {
				t.Errorf("ValidateRedirectUri(%q) = %v, want %v", c.redirectURI, got, c.want)
			}
		})
	}
}

func TestVerifyClientSecret(t *testing.T) {
	secretHash := token.HashToken("s3cret")
	confidential := domainoauth.Client{ID: "c1", SecretHash: secretHash}
	public := domainoauth.Client{ID: "c2", SecretHash: ""}

	cases := []struct {
		name     string
		client   domainoauth.Client
		provided string
		want     bool
	}{
		{"机密 client 正确 secret", confidential, "s3cret", true},
		{"机密 client 错误 secret", confidential, "wrong", false},
		{"机密 client 空 secret", confidential, "", false},
		{"公开 client（无 hash）→ 永远 false", public, "anything", false},
		{"公开 client 空 secret", public, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VerifyClientSecret(c.client, c.provided); got != c.want {
				t.Errorf("VerifyClientSecret = %v, want %v", got, c.want)
			}
		})
	}
}

func TestVerifyPKCE(t *testing.T) {
	// RFC 7636 附录 B 测试向量：verifier → S256 challenge。
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	cases := []struct {
		name          string
		codeVerifier  string
		codeChallenge string
		want          bool
	}{
		{"正确 verifier", verifier, challenge, true},
		{"错误 verifier", "wrong-verifier", challenge, false},
		{"空 verifier", "", challenge, false},
		{"空 challenge", verifier, "", false},
		{"大小写敏感（challenge 改大小写）", verifier, "e9melhoa2owvfremtjguchaoek1t8urwbugjsstw-cm", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VerifyPKCE(c.codeVerifier, c.codeChallenge); got != c.want {
				t.Errorf("VerifyPKCE = %v, want %v", got, c.want)
			}
		})
	}
}
