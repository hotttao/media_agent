package caller

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"media_agent/hertz_infra/authn/applicationcaller"
	"testing"
	"time"
)

func TestVerifyPlatformSignerAndRejectUntrustedAssertions(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := applicationcaller.New(key, "issuer")
	jwks, _ := json.Marshal(signer.PublicJWKS())
	v, err := New(Config{Issuer: "issuer", Audience: "app", JWKS: jwks})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signer.Sign(applicationcaller.Identity{ApplicationID: "app", UserID: "user", ExternalID: "ext", OrganizationID: "org", TeamID: "team", WorkspaceID: "workspace", SessionID: "session"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := v.Verify(raw, Binding{WorkspaceID: "workspace", SessionID: "session"})
	if err != nil || identity.ExternalID != "ext" {
		t.Fatalf("%+v %v", identity, err)
	}
	if _, err = v.Verify(raw, Binding{WorkspaceID: "other"}); err == nil {
		t.Fatal("cross-workspace assertion")
	}
	for _, change := range []func(*jwt.Token, *applicationcaller.Claims){
		func(t *jwt.Token, c *applicationcaller.Claims) { c.Issuer = "attacker" },
		func(t *jwt.Token, c *applicationcaller.Claims) { c.Audience = jwt.ClaimStrings{"other"} },
		func(t *jwt.Token, c *applicationcaller.Claims) {
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Second))
		},
		func(t *jwt.Token, c *applicationcaller.Claims) {
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
		},
		func(t *jwt.Token, c *applicationcaller.Claims) { c.NotBefore = nil },
		func(t *jwt.Token, c *applicationcaller.Claims) { c.Subject = "" },
		func(t *jwt.Token, c *applicationcaller.Claims) { t.Header["typ"] = "JWT" },
		func(t *jwt.Token, c *applicationcaller.Claims) { t.Header["kid"] = "unknown" },
		func(t *jwt.Token, c *applicationcaller.Claims) { t.Header["crit"] = []string{"unhandled"} },
	} {
		c := &applicationcaller.Claims{}
		tok, _, e := jwt.NewParser().ParseUnverified(raw, c)
		if e != nil {
			t.Fatal(e)
		}
		change(tok, c)
		bad, _ := tok.SignedString(key)
		if _, e = v.Verify(bad, Binding{}); e == nil {
			t.Fatal("accepted invalid assertion")
		}
	}
	wrongKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	c := &applicationcaller.Claims{}
	tok, _, _ := jwt.NewParser().ParseUnverified(raw, c)
	bad, _ := tok.SignedString(wrongKey)
	if _, err = v.Verify(bad, Binding{}); err == nil {
		t.Fatal("forged signature")
	}
}
