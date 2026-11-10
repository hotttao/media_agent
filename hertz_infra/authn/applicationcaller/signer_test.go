package applicationcaller

import (
	"crypto/rand"
	"crypto/rsa"
	"github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestCallerRequiresIdentityAndUsesDedicatedClaims(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	s, e := New(key, "https://gateway.example/application-caller")
	if e != nil {
		t.Fatal(e)
	}
	p := Identity{ApplicationID: "business", UserID: "user", ExternalID: "external", OrganizationID: "org", TeamID: "team", WorkspaceID: "workspace", SessionID: "session"}
	raw, e := s.Sign(p, time.Now().Add(20*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	c := &Claims{}
	tok, e := jwt.ParseWithClaims(raw, c, func(token *jwt.Token) (any, error) {
		if token.Header["typ"] != Type || token.Header["kid"] != s.PublicJWKS().Keys[0].Kid {
			t.Fatal("caller header")
		}
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience("business"), jwt.WithIssuer("https://gateway.example/application-caller"), jwt.WithExpirationRequired())
	if e != nil || !tok.Valid || c.ExternalID != "external" || c.WorkspaceID != "workspace" || time.Until(c.ExpiresAt.Time) > 21*time.Second {
		t.Fatal("invalid Caller", e)
	}
	if _, e = s.Sign(p, time.Now().Add(-time.Second)); e == nil {
		t.Fatal("expired authority accepted")
	}
	p.SessionID = ""
	if _, e = s.Sign(p, time.Time{}); e == nil {
		t.Fatal("missing session accepted")
	}
	var missing *Signer
	if _, e = missing.Sign(p, time.Time{}); e == nil {
		t.Fatal("missing signer accepted")
	}
}
