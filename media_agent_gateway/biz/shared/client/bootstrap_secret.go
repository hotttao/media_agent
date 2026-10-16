package client

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"media_agent/hertz_infra/sessioncredential"
)

// BootstrapCredential is private deployment material, never a resource Spec.
type BootstrapCredential struct {
	Secret    string
	ExpiresAt time.Time
}
type BootstrapFiles struct {
	Root string
	Keys *sessioncredential.Ring
}
type encryptedBootstrap struct{ KeyID, Ciphertext string }

var ErrBootstrap = errors.New("runtime bootstrap credential unavailable")

func (s BootstrapFiles) aead(scope, keyID string) (cipher.AEAD, error) {
	derived, err := s.Keys.Derive("docker-bootstrap-encryption-v1", keyID, scope)
	if err != nil {
		return nil, ErrBootstrap
	}
	key := sha256.Sum256([]byte(derived))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, ErrBootstrap
	}
	return cipher.NewGCM(block)
}
func (s BootstrapFiles) GetOrCreate(ctx context.Context, scope string, create func() (BootstrapCredential, error)) (BootstrapCredential, error) {
	if s.Root == "" || s.Keys == nil || scope == "" {
		return BootstrapCredential{}, ErrBootstrap
	}
	if err := os.MkdirAll(s.Root, 0700); err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	info, err := os.Lstat(s.Root)
	if err != nil || !info.IsDir() {
		return BootstrapCredential{}, ErrBootstrap
	}
	if info.Mode().Perm()&0077 != 0 {
		if err := os.Chmod(s.Root, 0700); err != nil {
			return BootstrapCredential{}, ErrBootstrap
		}
	}
	hash := sha256.Sum256([]byte(scope))
	path := filepath.Join(s.Root, hex.EncodeToString(hash[:]))
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if ctx.Err() != nil {
		return BootstrapCredential{}, ctx.Err()
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
			return BootstrapCredential{}, ErrBootstrap
		}
		data, err := os.ReadFile(path)
		var record encryptedBootstrap
		if err != nil || json.Unmarshal(data, &record) != nil {
			return BootstrapCredential{}, ErrBootstrap
		}
		sealed, err := base64.RawURLEncoding.DecodeString(record.Ciphertext)
		if err != nil {
			return BootstrapCredential{}, ErrBootstrap
		}
		aead, err := s.aead(scope, record.KeyID)
		if err != nil || len(sealed) < aead.NonceSize() {
			return BootstrapCredential{}, ErrBootstrap
		}
		plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(scope))
		if err != nil {
			return BootstrapCredential{}, ErrBootstrap
		}
		var value BootstrapCredential
		if json.Unmarshal(plain, &value) != nil || value.Secret == "" {
			return BootstrapCredential{}, ErrBootstrap
		}
		if value.ExpiresAt.After(time.Now().Add(5 * time.Minute)) {
			return value, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return BootstrapCredential{}, ErrBootstrap
	}
	value, err := create()
	if err != nil {
		return BootstrapCredential{}, err
	}
	if value.Secret == "" || !value.ExpiresAt.After(time.Now().Add(5*time.Minute)) {
		return BootstrapCredential{}, ErrBootstrap
	}
	plain, err := json.Marshal(value)
	if err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	aead, err := s.aead(scope, s.Keys.Active)
	if err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	sealed := aead.Seal(nonce, nonce, plain, []byte(scope))
	data, _ := json.Marshal(encryptedBootstrap{s.Keys.Active, base64.RawURLEncoding.EncodeToString(sealed)})
	f, err := os.CreateTemp(s.Root, ".bootstrap-")
	if err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return BootstrapCredential{}, ErrBootstrap
	}
	return value, nil
}
