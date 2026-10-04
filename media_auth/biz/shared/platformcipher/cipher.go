package platformcipher

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

var ErrUnavailable = errors.New("platform token encryption unavailable")

type Keyring struct {
	active string
	keys   map[string]cipher.AEAD
}
type Envelope struct {
	KeyVersion                   string
	TokenHash, Ciphertext, Nonce []byte
}

func New(active string, keys map[string][]byte) (*Keyring, error) {
	if !validVersion(active) || len(keys) == 0 || len(keys) > 16 {
		return nil, ErrUnavailable
	}
	r := &Keyring{active: active, keys: map[string]cipher.AEAD{}}
	for version, key := range keys {
		if !validVersion(version) || len(key) != 32 {
			return nil, ErrUnavailable
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, ErrUnavailable
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, ErrUnavailable
		}
		r.keys[version] = aead
	}
	if r.keys[active] == nil {
		return nil, ErrUnavailable
	}
	return r, nil
}

func LoadFile(path, version string) (*Keyring, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() != 32 {
		return nil, ErrUnavailable
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !os.SameFile(before, info) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 32 {
		return nil, ErrUnavailable
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	if err != nil || len(key) != 32 {
		return nil, ErrUnavailable
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()
	return New(version, map[string][]byte{version: key})
}

func (r *Keyring) Seal(id, user, secret string) (Envelope, error) {
	if r == nil || !validIdentity(id) || !validIdentity(user) || !validSecret(secret) || r.keys[r.active] == nil {
		return Envelope{}, ErrUnavailable
	}
	key := r.keys[r.active]
	nonce := make([]byte, key.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Envelope{}, ErrUnavailable
	}
	hash := sha256.Sum256([]byte(secret))
	return Envelope{KeyVersion: r.active, TokenHash: append([]byte(nil), hash[:]...), Nonce: nonce,
		Ciphertext: key.Seal(nil, nonce, []byte(secret), associatedData(id, user, r.active))}, nil
}

func (r *Keyring) Open(id, user string, t Envelope) (string, error) {
	if r == nil || !validIdentity(id) || !validIdentity(user) || !validVersion(t.KeyVersion) || len(t.TokenHash) != 32 || len(t.Ciphertext) > 4096 {
		return "", ErrUnavailable
	}
	key := r.keys[t.KeyVersion]
	if key == nil || len(t.Nonce) != key.NonceSize() || len(t.Ciphertext) < key.Overhead() {
		return "", ErrUnavailable
	}
	secret, err := key.Open(nil, t.Nonce, t.Ciphertext, associatedData(id, user, t.KeyVersion))
	if err != nil {
		return "", ErrUnavailable
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	hash := sha256.Sum256(secret)
	if !validSecret(string(secret)) || subtle.ConstantTimeCompare(hash[:], t.TokenHash) != 1 {
		return "", ErrUnavailable
	}
	return string(secret), nil
}

func associatedData(id, user, version string) []byte {
	return []byte("media_auth.platform_tokens.v1\x00" + id + "\x00" + user + "\x00" + version)
}

func validSecret(s string) bool {
	if len(s) != 68 || !strings.HasPrefix(s, "tos_") {
		return false
	}
	b, err := hex.DecodeString(s[4:])
	return err == nil && len(b) == 32
}

func validIdentity(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
func validVersion(s string) bool { return len(s) <= 64 && validIdentity(s) }
