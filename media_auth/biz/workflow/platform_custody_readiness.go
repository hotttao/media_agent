package workflow

import (
	"context"
	"errors"

	domain "media_agent/media_auth/biz/domain/platform_token"
	"media_agent/media_auth/biz/shared/platformcipher"
)

var ErrPlatformReadiness = errors.New("platform custody is not ready for cutover")

type PlatformCustodyReader interface {
	CustodyMigrationReady(context.Context) (bool, error)
	ReadCustodyPage(context.Context, string, int) ([]domain.Token, error)
}

// CheckPlatformCustody is read-only and includes revoked history. It neither
// migrates facts nor substitutes for stopping legacy writers before cutover.
func CheckPlatformCustody(ctx context.Context, reader PlatformCustodyReader, cipher *platformcipher.Keyring) error {
	if ctx == nil || reader == nil || cipher == nil || ctx.Err() != nil {
		return ErrPlatformReadiness
	}
	ready, err := reader.CustodyMigrationReady(ctx)
	if err != nil || !ready {
		return ErrPlatformReadiness
	}
	after := ""
	for {
		if ctx.Err() != nil {
			return ErrPlatformReadiness
		}
		page, err := reader.ReadCustodyPage(ctx, after, 100)
		if err != nil || len(page) > 100 {
			return ErrPlatformReadiness
		}
		if len(page) == 0 {
			return nil
		}
		for _, token := range page {
			if token.ID <= after || !token.ValidStorage() {
				return ErrPlatformReadiness
			}
			if _, err := cipher.Open(token.ID, token.UserID, platformcipher.Envelope{KeyVersion: token.KeyVersion, TokenHash: token.TokenHash, Ciphertext: token.Ciphertext, Nonce: token.Nonce}); err != nil {
				return ErrPlatformReadiness
			}
			after = token.ID
		}
	}
}
