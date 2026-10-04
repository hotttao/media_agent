package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/platform_token"
	"media_agent/media_auth/biz/shared/platformcipher"
)

type custodyReaderFake struct {
	ready  bool
	tokens []domain.Token
	fault  bool
	reads  int
}

func (f *custodyReaderFake) CustodyMigrationReady(context.Context) (bool, error) { return f.ready, nil }
func (f *custodyReaderFake) ReadCustodyPage(_ context.Context, after string, limit int) ([]domain.Token, error) {
	f.reads++
	if f.fault {
		return nil, errors.New("private diagnostic")
	}
	var page []domain.Token
	for _, token := range f.tokens {
		if token.ID > after {
			page = append(page, token)
			if len(page) == limit {
				break
			}
		}
	}
	return page, nil
}

func TestPlatformReadinessIncludesAllRevokedPagesAndRejectsBadCustody(t *testing.T) {
	ring, err := platformcipher.New("v1", map[string][]byte{"v1": make([]byte, 32)})
	if err != nil {
		t.Fatal("fixture ring failed")
	}
	f := &custodyReaderFake{ready: true}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 105; i++ {
		id := fmt.Sprintf("id-%04d", i)
		env, err := ring.Seal(id, "user", fmt.Sprintf("tos_%064x", i+1))
		if err != nil {
			t.Fatal("fixture seal failed")
		}
		f.tokens = append(f.tokens, domain.Token{ID: id, UserID: "user", KeyVersion: env.KeyVersion, TokenHash: env.TokenHash, Ciphertext: env.Ciphertext, Nonce: env.Nonce, CreatedAt: at, RevokedAt: &at})
	}
	if err := CheckPlatformCustody(t.Context(), f, ring); err != nil || f.reads != 3 {
		t.Fatal("complete revoked history not checked")
	}
	f.tokens[104].Ciphertext[0] ^= 1
	if err := CheckPlatformCustody(t.Context(), f, ring); !errors.Is(err, ErrPlatformReadiness) {
		t.Fatal("corrupt history passed")
	}
	f.ready = false
	before := f.reads
	if err := CheckPlatformCustody(t.Context(), f, ring); err == nil || f.reads != before {
		t.Fatal("unmigrated custody inspected or admitted")
	}
	f.ready, f.fault = true, true
	if err := CheckPlatformCustody(t.Context(), f, ring); !errors.Is(err, ErrPlatformReadiness) {
		t.Fatal("read fault admitted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := CheckPlatformCustody(ctx, f, ring); err == nil {
		t.Fatal("cancelled readiness admitted")
	}
}
