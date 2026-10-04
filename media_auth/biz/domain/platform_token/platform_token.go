package platformtoken

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("platform token not found")
	ErrConflict = errors.New("platform token conflict")
	ErrInvalid  = errors.New("invalid platform token record")
)

// Token contains encrypted custody data only. Plaintext and encryption keys
// are transient application inputs, never fields in this persisted aggregate.
type Token struct {
	ID, UserID, KeyVersion       string
	TokenHash, Ciphertext, Nonce []byte
	CreatedAt                    time.Time
	ExpiresAt, RevokedAt         *time.Time
}

func (t Token) ActiveAt(now time.Time) bool {
	return t.RevokedAt == nil && (t.ExpiresAt == nil || now.Before(*t.ExpiresAt))
}

func (t Token) ValidStorage() bool {
	return validIdentity(t.ID) && validIdentity(t.UserID) && ValidKeyVersion(t.KeyVersion) && len(t.TokenHash) == 32 && len(t.Nonce) == 12 && len(t.Ciphertext) >= 17 && len(t.Ciphertext) <= 4096 && !t.CreatedAt.IsZero()
}

func ValidIdentity(s string) bool { return validIdentity(s) }
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

func ValidKeyVersion(s string) bool { return len(s) <= 64 && validIdentity(s) }

type Repository interface {
	Create(context.Context, Token) error
	// GetCurrent includes expired but unrevoked rows, so callers can revoke
	// them before replacing the single current credential.
	GetCurrent(context.Context, string) (Token, error)
	GetByHash(context.Context, []byte) (Token, error)
	RevokeForUser(context.Context, string, time.Time) error
}
