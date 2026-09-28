package workflow

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
	"media_agent/media_auth/biz/shared/ids"
)

type RotationSecrets interface {
	RotateWithMetadata(context.Context, string, []string, map[string]any) (service.IssuedSecret, error)
	Revoke(context.Context, string) error
	Get(context.Context, string) (service.SecretMetadata, error)
	List(context.Context, string, string) (service.RotationKeyPage, error)
}

var ErrRotationIncomplete = errors.New("api token rotation could not be completed; credential cleanup is pending")

type APITokenRotation struct {
	repo    domain.RotationRepository
	secrets RotationSecrets
	tx      TransactionRunner
	now     func() time.Time
}

func NewAPITokenRotation(repo domain.RotationRepository, secrets RotationSecrets, tx TransactionRunner) *APITokenRotation {
	return &APITokenRotation{repo: repo, secrets: secrets, tx: tx, now: time.Now}
}

func (w *APITokenRotation) Rotate(ctx context.Context, token domain.Token) (service.CreateResult, error) {
	now := w.now()
	operation := domain.Rotation{ID: ids.New(), TokenID: token.ID, OldKeyID: token.TalosKeyID, ActorID: token.ActorID,
		Status: domain.RotationPending, CreatedAt: now, Deadline: now.Add(30 * time.Second)}
	if err := w.tx.RunInTx(ctx, func(txCtx context.Context) error { return w.repo.BeginRotation(txCtx, token, operation) }); err != nil {
		return service.CreateResult{}, err
	}
	issued, err := w.secrets.RotateWithMetadata(ctx, token.TalosKeyID, token.Scopes, map[string]any{
		"api_token_id": token.ID, "rotation_id": operation.ID, "subject_user_id": token.SubjectUserID, "organization_id": token.OrganizationID,
	})
	if err == nil {
		actual, expected := slices.Clone(issued.Scopes), slices.Clone(token.Scopes)
		slices.Sort(actual)
		slices.Sort(expected)
		if issued.Secret == "" || issued.KeyID == "" || issued.KeyID == token.TalosKeyID || issued.ActorID != token.ActorID || !slices.Equal(actual, expected) {
			err = ErrRotationIncomplete
		}
	}
	var rotated domain.Token
	if err == nil {
		expiry := token.ExpiresAt
		if !issued.ExpiresAt.IsZero() {
			expiry = &issued.ExpiresAt
		}
		err = w.tx.RunInTx(ctx, func(txCtx context.Context) error {
			var commitErr error
			rotated, commitErr = w.repo.CommitRotation(txCtx, operation, issued.KeyID, expiry, w.now())
			return commitErr
		})
	}
	if err != nil {
		// Request cancellation cannot erase cleanup intent. This bounded attempt
		// only accelerates durable recovery; failure is retried after restart.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		_ = w.Recover(cleanupCtx, operation, true)
		cancel()
		return service.CreateResult{}, err
	}
	return service.CreateResult{Token: rotated, Secret: issued.Secret}, nil
}

func (w *APITokenRotation) revokeAndConfirm(ctx context.Context, keyID string) error {
	if err := w.secrets.Revoke(ctx, keyID); err != nil {
		metadata, queryErr := w.secrets.Get(ctx, keyID)
		if queryErr != nil || metadata.KeyID != keyID || metadata.Status != "KEY_STATUS_REVOKED" {
			return err
		}
	}
	return nil
}

func (w *APITokenRotation) Recover(ctx context.Context, operation domain.Rotation, force bool) error {
	claimed, err := w.repo.ClaimRotationRecovery(ctx, operation.ID, w.now(), force)
	if err != nil || !claimed {
		return err
	}
	// Fence late Rotate calls before enumerating replacements. Talos performs
	// rotation atomically and rejects a revoked old key.
	if err := w.revokeAndConfirm(ctx, operation.OldKeyID); err != nil {
		return err
	}
	pageToken := ""
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[pageToken] {
			return ErrRotationIncomplete
		}
		seen[pageToken] = true
		page, err := w.secrets.List(ctx, operation.ActorID, pageToken)
		if err != nil {
			return err
		}
		for _, key := range page.Keys {
			if key.KeyID == "" || key.ActorID != operation.ActorID {
				return ErrRotationIncomplete
			}
			if key.Metadata["api_token_id"] != operation.TokenID || key.Metadata["rotation_id"] != operation.ID {
				continue
			}
			if key.Status != "KEY_STATUS_REVOKED" {
				if err := w.revokeAndConfirm(ctx, key.KeyID); err != nil {
					return err
				}
			}
		}
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}
	return w.tx.RunInTx(ctx, func(txCtx context.Context) error { return w.repo.AbortRotation(txCtx, operation, w.now()) })
}

func (w *APITokenRotation) ProcessBatch(ctx context.Context, afterID string) (string, error) {
	operations, err := w.repo.ListPendingRotations(ctx, afterID, w.now(), 100)
	if err != nil {
		return afterID, err
	}
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return afterID, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := w.Recover(attemptCtx, operation, false)
		cancel()
		if err != nil {
			log.Printf("api token rotation recovery pending operation_id=%s", operation.ID)
		}
		afterID = operation.ID
	}
	if len(operations) < 100 {
		return "", nil
	}
	return afterID, nil
}

func (w *APITokenRotation) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		next, err := w.ProcessBatch(ctx, cursor)
		if err != nil && ctx.Err() == nil {
			log.Print("api token rotation recovery scan failed")
		}
		cursor = next
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
