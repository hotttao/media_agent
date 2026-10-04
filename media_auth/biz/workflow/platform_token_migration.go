package workflow

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	platform "media_agent/media_auth/biz/domain/platform_token"
	legacy "media_agent/media_auth/biz/domain/service_token"
	"media_agent/media_auth/biz/shared/platformcipher"
)

var ErrPlatformMigration = errors.New("platform token migration failed; no verified commit")
var errPlatformDryRun = errors.New("platform token migration dry run")

type platformMigrationTarget interface {
	Create(context.Context, platform.Token) error
	GetIncludingRevoked(context.Context, string) (platform.Token, error)
}

type PlatformMigrationResult struct {
	Verified  int  `json:"verified"`
	Created   int  `json:"created"`
	Reused    int  `json:"reused"`
	Committed bool `json:"committed"`
}

type PlatformTokenMigration struct {
	tx     TransactionRunner
	source legacy.PlatformMigrationSource
	target platformMigrationTarget
	cipher *platformcipher.Keyring
}

func NewPlatformTokenMigration(tx TransactionRunner, source legacy.PlatformMigrationSource, target platformMigrationTarget, cipher *platformcipher.Keyring) *PlatformTokenMigration {
	return &PlatformTokenMigration{tx: tx, source: source, target: target, cipher: cipher}
}

func (w *PlatformTokenMigration) Run(ctx context.Context, apply bool) (PlatformMigrationResult, error) {
	var result PlatformMigrationResult
	if w == nil || w.tx == nil || w.source == nil || w.target == nil || w.cipher == nil {
		return result, ErrPlatformMigration
	}
	err := w.tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := w.source.LockPlatformMigration(ctx); err != nil {
			return err
		}
		after := ""
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			page, err := w.source.ReadPlatformMigrationPage(ctx, after, 100)
			if err != nil {
				return err
			}
			if len(page) == 0 {
				break
			}
			for _, old := range page {
				if old.ID <= after {
					return ErrPlatformMigration
				}
				env, err := w.cipher.Seal(old.ID, old.UserID, old.Secret)
				if err != nil || old.CreatedAt.IsZero() {
					return ErrPlatformMigration
				}
				hash, err := hex.DecodeString(old.Hash)
				if err != nil || len(hash) != sha256.Size || subtle.ConstantTimeCompare(hash, env.TokenHash) != 1 {
					return ErrPlatformMigration
				}
				saved, err := w.target.GetIncludingRevoked(ctx, old.ID)
				if errors.Is(err, platform.ErrNotFound) {
					draft := platform.Token{ID: old.ID, UserID: old.UserID, TokenHash: env.TokenHash, Ciphertext: env.Ciphertext, Nonce: env.Nonce, KeyVersion: env.KeyVersion, CreatedAt: old.CreatedAt, ExpiresAt: old.ExpiresAt, RevokedAt: old.RevokedAt}
					if err := w.target.Create(ctx, draft); err != nil {
						return err
					}
					result.Created++
					saved, err = w.target.GetIncludingRevoked(ctx, old.ID)
				} else if err == nil {
					result.Reused++
				}
				if err != nil || !sameLegacyFacts(old, saved, hash) {
					return ErrPlatformMigration
				}
				secret, err := w.cipher.Open(saved.ID, saved.UserID, platformcipher.Envelope{KeyVersion: saved.KeyVersion, TokenHash: saved.TokenHash, Ciphertext: saved.Ciphertext, Nonce: saved.Nonce})
				if err != nil || subtle.ConstantTimeCompare([]byte(secret), []byte(old.Secret)) != 1 {
					return ErrPlatformMigration
				}
				if err := w.source.DeleteMigratedPlatform(ctx, old.ID); err != nil {
					return err
				}
				result.Verified++
				after = old.ID
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !apply {
			return errPlatformDryRun
		}
		return nil
	})
	if !apply && errors.Is(err, errPlatformDryRun) {
		return result, nil
	}
	if err != nil {
		return PlatformMigrationResult{}, ErrPlatformMigration
	}
	result.Committed = true
	return result, nil
}

func sameLegacyFacts(old legacy.LegacyPlatformRecord, saved platform.Token, hash []byte) bool {
	return old.ID == saved.ID && old.UserID == saved.UserID && subtle.ConstantTimeCompare(hash, saved.TokenHash) == 1 && old.CreatedAt.Equal(saved.CreatedAt) && equalOptionalTime(old.ExpiresAt, saved.ExpiresAt) && equalOptionalTime(old.RevokedAt, saved.RevokedAt)
}

func equalOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
