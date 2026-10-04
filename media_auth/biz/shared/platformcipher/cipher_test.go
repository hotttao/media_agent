package platformcipher

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSecret = "tos_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newTestKeyring(t *testing.T) *Keyring {
	t.Helper()
	r, err := New("v1", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPlatformCipherUsesFreshNonceAndBindsCustodyIdentity(t *testing.T) {
	r := newTestKeyring(t)
	a, err := r.Seal("token", "owner", testSecret)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Seal("token", "owner", testSecret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a.Nonce, b.Nonce) || bytes.Equal(a.Ciphertext, b.Ciphertext) || bytes.Contains(a.Ciphertext, []byte(testSecret)) {
		t.Fatal("plaintext persisted or encryption nonce reused")
	}
	for _, e := range []Envelope{a, b} {
		if got, err := r.Open("token", "owner", e); err != nil || got != testSecret {
			t.Fatal("custody round-trip failed")
		}
	}
	for _, tc := range []struct {
		name, id, user string
		mutate         func(*Envelope)
	}{
		{"wrong ID", "other", "owner", func(*Envelope) {}},
		{"wrong owner", "token", "other", func(*Envelope) {}},
		{"ciphertext modified", "token", "owner", func(e *Envelope) { e.Ciphertext[0] ^= 1 }},
		{"nonce modified", "token", "owner", func(e *Envelope) { e.Nonce[0] ^= 1 }},
		{"hash modified", "token", "owner", func(e *Envelope) { e.TokenHash[0] ^= 1 }},
		{"unknown version", "token", "owner", func(e *Envelope) { e.KeyVersion = "unknown" }},
		{"short nonce", "token", "owner", func(e *Envelope) { e.Nonce = nil }},
		{"short ciphertext", "token", "owner", func(e *Envelope) { e.Ciphertext = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := a
			e.Ciphertext = append([]byte(nil), a.Ciphertext...)
			e.Nonce = append([]byte(nil), a.Nonce...)
			e.TokenHash = append([]byte(nil), a.TokenHash...)
			tc.mutate(&e)
			if got, err := r.Open(tc.id, tc.user, e); got != "" || !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), testSecret) {
				t.Fatal("corrupted custody admitted or secret leaked")
			}
		})
	}
}

func TestPlatformCipherRetainsOldKeyVersionsAndCopiesKeyMaterial(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	first, err := New("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatal(err)
	}
	e, err := first.Seal("token", "owner", testSecret)
	if err != nil {
		t.Fatal(err)
	}
	for i := range key {
		key[i] = 0
	}
	if _, err := first.Open("token", "owner", e); err != nil {
		t.Fatal("caller mutation changed loaded key")
	}
	rotated, err := New("v2", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32), "v2": bytes.Repeat([]byte{2}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := rotated.Open("token", "owner", e); err != nil || got != testSecret {
		t.Fatal("old key cannot recover existing custody")
	}
	next, err := rotated.Seal("token", "owner", testSecret)
	if err != nil || next.KeyVersion != "v2" {
		t.Fatal("active key was not used")
	}
	alias, err := New("alias", map[string][]byte{"v1": bytes.Repeat([]byte{1}, 32), "alias": bytes.Repeat([]byte{1}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	e.KeyVersion = "alias"
	if _, err := alias.Open("token", "owner", e); !errors.Is(err, ErrUnavailable) {
		t.Fatal("key version was not cryptographically bound")
	}
}

func TestPlatformCipherRejectsUnsafeKeyConfigurationAndSecretFiles(t *testing.T) {
	for _, key := range [][]byte{nil, make([]byte, 16), make([]byte, 31), make([]byte, 33)} {
		if _, err := New("v1", map[string][]byte{"v1": key}); !errors.Is(err, ErrUnavailable) {
			t.Fatal("non-AES256 key accepted")
		}
	}
	if _, err := New("missing", map[string][]byte{"v1": make([]byte, 32)}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("missing active version accepted")
	}
	r := newTestKeyring(t)
	for _, secret := range []string{"", "private-secret", "tos_" + strings.Repeat("z", 64)} {
		if _, err := r.Seal("token", "owner", secret); !errors.Is(err, ErrUnavailable) {
			t.Fatal("invalid secret accepted")
		}
	}
	for _, id := range []string{"", "user\x00other", strings.Repeat("a", 129)} {
		if _, err := r.Seal(id, "owner", testSecret); !errors.Is(err, ErrUnavailable) {
			t.Fatal("ambiguous identity accepted")
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "private.key")
	if err := os.WriteFile(path, bytes.Repeat([]byte{1}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, "v1"); err != nil {
		t.Fatal("private key rejected")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path, "v1"); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), path) {
		t.Fatal("unsafe key permissions or path exposed")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.key")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(link, "v1"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("symlink secret accepted")
	}
	if _, err := LoadFile(dir, "v1"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("directory accepted as key")
	}
}
