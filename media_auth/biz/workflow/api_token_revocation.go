package workflow

import (
	"context"
	"log"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
)

type APITokenRevoker interface {
	Revoke(context.Context, string, string) error
}

// APITokenRevocation resumes durable revoking records after request cancellation
// or process restart. It never needs, persists or logs a Secret.
type APITokenRevocation struct {
	source  domain.RevocationSource
	revoker APITokenRevoker
}

func NewAPITokenRevocation(source domain.RevocationSource, revoker APITokenRevoker) *APITokenRevocation {
	return &APITokenRevocation{source: source, revoker: revoker}
}

// ProcessBatch returns a keyset cursor even when individual keys fail. One
// unavailable key must not starve the other pending keys. An empty cursor starts
// another sweep; the cursor itself need not survive process restarts.
func (w *APITokenRevocation) ProcessBatch(ctx context.Context, afterID string) (string, error) {
	tokens, err := w.source.ListRevoking(ctx, afterID, 100)
	if err != nil {
		return afterID, err
	}
	for _, token := range tokens {
		if err := ctx.Err(); err != nil {
			return afterID, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := w.revoker.Revoke(attemptCtx, token.SubjectUserID, token.ID)
		cancel()
		if err != nil {
			// Deliberately omit upstream errors: future adapters must not leak
			// credentials into background logs through an echoed HTTP body.
			log.Printf("api token revocation pending token_id=%s", token.ID)
		}
		afterID = token.ID
	}
	if len(tokens) < 100 {
		return "", nil
	}
	return afterID, nil
}

func (w *APITokenRevocation) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		next, err := w.ProcessBatch(ctx, cursor)
		if err != nil && ctx.Err() == nil {
			log.Print("api token revocation recovery scan failed")
		}
		cursor = next
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
