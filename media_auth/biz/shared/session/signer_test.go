// signer_test.go：session JWT signer 单测。
package session

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/domain/auth"
)

func TestSignerRS256AndLegacyDualVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(t, key)})
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	if err := os.WriteFile(privatePath, privatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, publicPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	rs, err := NewSigner(&configpb.AuthConfig{JwtSecret: "legacy", JwtPrivateKeyFile: privatePath, JwtPublicKeyFile: publicPath, JwtKeyId: "key-1", AcceptLegacyHs256: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := rs.Sign(auth.Session{Sub: "u1", AuthVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := rs.Verify(token); err != nil || got.Sub != "u1" || got.AuthVersion != 2 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if jwk := rs.JWK(); jwk == nil || jwk.Kid != "key-1" || jwk.Alg != "RS256" || jwk.N == "" {
		t.Fatalf("jwk=%+v", jwk)
	}
	legacy, _ := NewSigner(&configpb.AuthConfig{JwtSecret: "legacy"})
	legacyToken, _ := legacy.Sign(auth.Session{Sub: "old"})
	if _, err := rs.Verify(legacyToken); err != nil {
		t.Fatalf("dual verify legacy: %v", err)
	}
	rs.acceptLegacy = false
	if _, err := rs.Verify(legacyToken); err == nil {
		t.Fatal("legacy token accepted after dual-verification window")
	}
}

func mustPKCS8(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	value, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestSigner_SignVerify(t *testing.T) {
	s, err := NewSigner(&configpb.AuthConfig{JwtSecret: "test-secret", JwtExpiresInSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	sess := auth.Session{Sub: "u1", Username: "alice", Name: "Alice", Role: "user"}
	tok, err := s.Sign(sess)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Sub != "u1" || got.Username != "alice" || got.Role != "user" {
		t.Errorf("got session=%+v", got)
	}
	if got.Exp.IsZero() || time.Now().After(got.Exp) {
		t.Errorf("exp invalid: %v", got.Exp)
	}
}

func TestSigner_VerifyBadToken(t *testing.T) {
	s, _ := NewSigner(&configpb.AuthConfig{JwtSecret: "test-secret"})
	if _, err := s.Verify("not-a-jwt"); err == nil {
		t.Errorf("want error for bad token")
	}
}

func TestSigner_VerifyWrongSecret(t *testing.T) {
	s1, _ := NewSigner(&configpb.AuthConfig{JwtSecret: "secret1"})
	s2, _ := NewSigner(&configpb.AuthConfig{JwtSecret: "secret2"})
	tok, _ := s1.Sign(auth.Session{Sub: "u1"})
	if _, err := s2.Verify(tok); err == nil {
		t.Errorf("want error for wrong secret")
	}
}

func TestSigner_ShouldRenew(t *testing.T) {
	s, _ := NewSigner(&configpb.AuthConfig{JwtSecret: "x", JwtExpiresInSeconds: 3600}) // 半周期 1800s
	// 剩余 > 半周期 → 不续期
	far := auth.Session{Exp: time.Now().Add(3000 * time.Second)}
	if s.ShouldRenew(far) {
		t.Errorf("far exp should not renew")
	}
	// 剩余 < 半周期 → 续期
	near := auth.Session{Exp: time.Now().Add(100 * time.Second)}
	if !s.ShouldRenew(near) {
		t.Errorf("near exp should renew")
	}
}

func TestNewSigner_RequiresSecret(t *testing.T) {
	if _, err := NewSigner(nil); err == nil {
		t.Errorf("want error for nil cfg")
	}
	if _, err := NewSigner(&configpb.AuthConfig{JwtSecret: ""}); err == nil {
		t.Errorf("want error for empty secret")
	}
}
