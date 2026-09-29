package workflow

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
)

type IssuanceSecrets interface {
	Issue(context.Context, service.IssueSecretCommand) (service.IssuedSecret, error)
	RecoverIssue(context.Context, service.IssueSecretCommand) (service.IssuanceKey, error)
	List(context.Context, string, string) (service.RotationKeyPage, error)
	Revoke(context.Context, string) error
	Get(context.Context, string) (service.SecretMetadata, error)
}

var ErrIssuanceIncomplete = errors.New("api token issuance could not be completed; credential cleanup is pending")

type APITokenIssuance struct {
	repo    domain.IssuanceRepository
	secrets IssuanceSecrets
	tx      TransactionRunner
	now     func() time.Time
}

func NewAPITokenIssuance(repo domain.IssuanceRepository, secrets IssuanceSecrets, tx TransactionRunner) *APITokenIssuance {
	return &APITokenIssuance{repo: repo, secrets: secrets, tx: tx, now: time.Now}
}

func issuanceMetadata(token domain.Token) map[string]any {
	return map[string]any{"api_token_id": token.ID, "issuance_id": token.ID,
		"subject_user_id": token.SubjectUserID, "organization_id": token.OrganizationID}
}

func (w *APITokenIssuance) Issue(ctx context.Context, token domain.Token) (service.CreateResult, error) {
	now := w.now()
	if token.ID == "" || token.ExpiresAt == nil || !token.ExpiresAt.After(now) {
		return service.CreateResult{}, service.ErrInvalidRequest
	}
	deadline := now.Add(30 * time.Second)
	token.Status, token.TalosKeyID, token.IssuanceDeadline = domain.StatusIssuing, "", &deadline
	var err error
	// This commit must precede Talos. A failed local insert has no external side effects.
	err = w.tx.RunInTx(ctx, func(txCtx context.Context) error {
		token, err = w.repo.Create(txCtx, token)
		return err
	})
	if err != nil {
		return service.CreateResult{}, err
	}
	ttl := token.ExpiresAt.Sub(now).Round(time.Second)
	if ttl < time.Second {
		ttl = time.Second
	}
	issued, err := w.secrets.Issue(ctx, service.IssueSecretCommand{Name: token.Name, ActorID: token.ActorID,
		RequestID: token.ID, TTL: ttl.String(), Scopes: token.Scopes, Metadata: issuanceMetadata(token)})
	if err == nil {
		actual, expected := slices.Clone(issued.Scopes), slices.Clone(token.Scopes)
		slices.Sort(actual)
		slices.Sort(expected)
		if issued.KeyID == "" || issued.Secret == "" || issued.ActorID != token.ActorID || !slices.Equal(actual, expected) {
			err = ErrIssuanceIncomplete
		}
	}
	var active domain.Token
	if err == nil {
		expiry := token.ExpiresAt
		if !issued.ExpiresAt.IsZero() {
			expiry = &issued.ExpiresAt
		}
		err = w.tx.RunInTx(ctx, func(txCtx context.Context) error {
			var commitErr error
			active, commitErr = w.repo.CommitIssuance(txCtx, token.ID, issued.KeyID, expiry, w.now())
			return commitErr
		})
	}
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		_ = w.Recover(cleanupCtx, token.ID, true)
		cancel()
		return service.CreateResult{}, err
	}
	return service.CreateResult{Token: active, Secret: issued.Secret}, nil
}

func (w *APITokenIssuance) Recover(ctx context.Context, id string, force bool) error {
	var token domain.Token
	var claimed bool
	err := w.tx.RunInTx(ctx, func(txCtx context.Context) error {
		var claimErr error
		token, claimed, claimErr = w.repo.ClaimIssuanceCleanup(txCtx, id, w.now(), force)
		return claimErr
	})
	if err != nil || !claimed {
		return err
	}
	var found service.IssuanceKey
	cursor := ""
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[cursor] {
			return ErrIssuanceIncomplete
		}
		seen[cursor] = true
		page, err := w.secrets.List(ctx, token.ActorID, cursor)
		if err != nil {
			return err
		}
		for _, key := range page.Keys {
			if key.KeyID == "" || key.ActorID != token.ActorID {
				return ErrIssuanceIncomplete
			}
			if key.Metadata["api_token_id"] != token.ID || key.Metadata["issuance_id"] != token.ID {
				continue
			}
			if found.KeyID != "" && found.KeyID != key.KeyID {
				return ErrIssuanceIncomplete
			}
			found = service.IssuanceKey{KeyID: key.KeyID, ActorID: key.ActorID, Status: key.Status, Metadata: key.Metadata}
		}
		if page.NextPageToken == "" {
			break
		}
		cursor = page.NextPageToken
	}
	if found.KeyID == "" {
		// List alone cannot fence a late original Issue. Replaying the same
		// trusted request ID resolves one unique key, with Secret discarded.
		found, err = w.secrets.RecoverIssue(ctx, service.IssueSecretCommand{Name: "cleanup-" + token.ID,
			ActorID: token.ActorID, RequestID: token.ID, Scopes: token.Scopes, Metadata: issuanceMetadata(token)})
		if err != nil {
			return err
		}
	}
	if found.KeyID == "" || found.ActorID != token.ActorID || found.Metadata["api_token_id"] != token.ID || found.Metadata["issuance_id"] != token.ID {
		return ErrIssuanceIncomplete
	}
	if found.Status != "KEY_STATUS_REVOKED" {
		if err := w.secrets.Revoke(ctx, found.KeyID); err != nil {
			metadata, queryErr := w.secrets.Get(ctx, found.KeyID)
			if queryErr != nil || metadata.KeyID != found.KeyID || metadata.Status != "KEY_STATUS_REVOKED" {
				return err
			}
		}
	}
	return w.tx.RunInTx(ctx, func(txCtx context.Context) error {
		return w.repo.FinishIssuanceCleanup(txCtx, token.ID, found.KeyID, w.now())
	})
}

func (w *APITokenIssuance) ProcessBatch(ctx context.Context, afterID string) (string, error) {
	tokens, err := w.repo.ListPendingIssuances(ctx, afterID, w.now(), 100)
	if err != nil {
		return afterID, err
	}
	for _, token := range tokens {
		if err := ctx.Err(); err != nil {
			return afterID, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := w.Recover(attemptCtx, token.ID, false)
		cancel()
		if err != nil {
			log.Printf("api token issuance recovery pending token_id=%s", token.ID)
		}
		afterID = token.ID
	}
	if len(tokens) < 100 {
		return "", nil
	}
	return afterID, nil
}

func (w *APITokenIssuance) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		next, err := w.ProcessBatch(ctx, cursor)
		if err != nil && ctx.Err() == nil {
			log.Print("api token issuance recovery scan failed")
		}
		cursor = next
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
