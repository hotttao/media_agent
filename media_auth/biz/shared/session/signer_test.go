// signer_test.go：session JWT signer 单测。
package session

import (
	"testing"
	"time"

	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/domain/auth"
)

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
