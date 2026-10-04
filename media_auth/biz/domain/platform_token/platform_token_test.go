package platformtoken

import (
	"testing"
	"time"
)

func TestPlatformTokenLifecycleAndStorageShape(t *testing.T) {
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	token := Token{ID: "id", UserID: "owner", KeyVersion: "v1", TokenHash: make([]byte, 32), Nonce: make([]byte, 12), Ciphertext: make([]byte, 84), CreatedAt: now}
	if !token.ValidStorage() || !token.ActiveAt(now) {
		t.Fatal("valid custody rejected")
	}
	expired := now
	token.ExpiresAt = &expired
	if token.ActiveAt(now) {
		t.Fatal("expiry boundary accepted")
	}
	token.ExpiresAt = nil
	token.RevokedAt = &now
	if token.ActiveAt(now) || !token.ValidStorage() {
		t.Fatal("revoked history was lost or accepted")
	}
	for _, change := range []func(*Token){func(v *Token) { v.UserID = "" }, func(v *Token) { v.ID = "bad\x00id" }, func(v *Token) { v.TokenHash = nil }, func(v *Token) { v.Nonce = make([]byte, 8) }, func(v *Token) { v.KeyVersion = "" }, func(v *Token) { v.CreatedAt = time.Time{} }} {
		v := token
		change(&v)
		if v.ValidStorage() {
			t.Fatal("invalid storage shape accepted")
		}
	}
}
