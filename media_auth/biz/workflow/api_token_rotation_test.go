package workflow

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/api_token"
	service "media_agent/media_auth/biz/service/api_token"
)

type rotationTx struct{}

func (rotationTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type rotationRepo struct {
	token                         domain.Token
	operation                     domain.Rotation
	beginErr, commitErr, abortErr error
}

func (r *rotationRepo) BeginRotation(_ context.Context, token domain.Token, operation domain.Rotation) error {
	if r.beginErr != nil {
		return r.beginErr
	}
	if r.token.Status != domain.StatusActive {
		return domain.ErrConflict
	}
	r.token.Status, r.operation = domain.StatusRotating, operation
	return nil
}
func (r *rotationRepo) CommitRotation(_ context.Context, operation domain.Rotation, key string, expiry *time.Time, at time.Time) (domain.Token, error) {
	if r.commitErr != nil {
		return domain.Token{}, r.commitErr
	}
	if r.operation.Status != domain.RotationPending || r.token.Status != domain.StatusRotating || !at.Before(r.operation.Deadline) {
		return domain.Token{}, domain.ErrConflict
	}
	r.operation.Status = domain.RotationCompleted
	r.token.Status, r.token.TalosKeyID, r.token.ExpiresAt = domain.StatusActive, key, expiry
	return r.token, nil
}
func (r *rotationRepo) ClaimRotationRecovery(_ context.Context, id string, at time.Time, force bool) (bool, error) {
	if r.operation.ID != id || r.operation.Status == domain.RotationCompleted || r.operation.Status == domain.RotationAborted {
		return false, nil
	}
	if r.operation.Status == domain.RotationPending && !force && at.Before(r.operation.Deadline) {
		return false, nil
	}
	r.operation.Status = domain.RotationRecovering
	return true, nil
}
func (r *rotationRepo) AbortRotation(context.Context, domain.Rotation, time.Time) error {
	if r.abortErr != nil {
		return r.abortErr
	}
	r.operation.Status, r.token.Status = domain.RotationAborted, domain.StatusRevoked
	return nil
}
func (r *rotationRepo) ListPendingRotations(_ context.Context, afterID string, at time.Time, _ int) ([]domain.Rotation, error) {
	if r.operation.ID <= afterID {
		return nil, nil
	}
	if r.operation.Status == domain.RotationRecovering || (r.operation.Status == domain.RotationPending && !at.Before(r.operation.Deadline)) {
		return []domain.Rotation{r.operation}, nil
	}
	return nil, nil
}

type rotationSecrets struct {
	repo                          *rotationRepo
	issued                        service.IssuedSecret
	rotateErr, listErr, revokeErr error
	calls                         []string
	metadata                      map[string]any
	pages                         map[string]service.RotationKeyPage
	onRotate                      func(context.Context)
	confirmedRevoke               bool
}

func (s *rotationSecrets) RotateWithMetadata(ctx context.Context, _ string, _ []string, metadata map[string]any) (service.IssuedSecret, error) {
	s.calls = append(s.calls, "rotate")
	s.metadata = metadata
	if s.repo.token.Status != domain.StatusRotating || s.repo.operation.Status != domain.RotationPending {
		return service.IssuedSecret{}, errors.New("called before durable intent")
	}
	if s.onRotate != nil {
		s.onRotate(ctx)
	}
	return s.issued, s.rotateErr
}
func (s *rotationSecrets) Revoke(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.calls = append(s.calls, "revoke:"+key)
	return s.revokeErr
}
func (s *rotationSecrets) Get(_ context.Context, key string) (service.SecretMetadata, error) {
	s.calls = append(s.calls, "get:"+key)
	if s.confirmedRevoke {
		return service.SecretMetadata{KeyID: key, Status: "KEY_STATUS_REVOKED"}, nil
	}
	return service.SecretMetadata{}, errors.New("unavailable")
}
func (s *rotationSecrets) List(_ context.Context, _ string, cursor string) (service.RotationKeyPage, error) {
	s.calls = append(s.calls, "list:"+cursor)
	if s.listErr != nil {
		return service.RotationKeyPage{}, s.listErr
	}
	if s.pages != nil {
		return s.pages[cursor], nil
	}
	return service.RotationKeyPage{Keys: []service.RotationKey{{KeyID: "new-key", ActorID: s.repo.token.ActorID, Status: "KEY_STATUS_ACTIVE", Metadata: s.metadata}}}, nil
}
func newRotation(t *testing.T) (*APITokenRotation, *rotationRepo, *rotationSecrets) {
	t.Helper()
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	expiry := now.Add(time.Hour)
	repo := &rotationRepo{token: domain.Token{ID: "token", TalosKeyID: "old-key", SubjectUserID: "owner", ActorID: "User:owner", Status: domain.StatusActive, Scopes: []string{"api.access"}, ExpiresAt: &expiry}}
	secrets := &rotationSecrets{repo: repo, issued: service.IssuedSecret{KeyID: "new-key", ActorID: "User:owner", Scopes: []string{"api.access"}, Secret: "one-time"}}
	flow := NewAPITokenRotation(repo, secrets, rotationTx{})
	flow.now = func() time.Time { return now }
	return flow, repo, secrets
}
func TestRotationCommitsBeforeReturningSecretAndNeverRecoversCompleted(t *testing.T) {
	flow, repo, secrets := newRotation(t)
	result, err := flow.Rotate(t.Context(), repo.token)
	if err != nil || result.Secret != "one-time" || result.Token.TalosKeyID != "new-key" || repo.operation.Status != domain.RotationCompleted {
		t.Fatal("rotation did not commit before returning secret")
	}
	if secrets.metadata["api_token_id"] != "token" || secrets.metadata["rotation_id"] != repo.operation.ID {
		t.Fatal("replacement lacks recovery identity")
	}
	if err := flow.Recover(t.Context(), repo.operation, true); err != nil || !reflect.DeepEqual(secrets.calls, []string{"rotate"}) {
		t.Fatal("completed rotation was compensated")
	}
}
func TestRotationFailureCleansOldAndReplacementWithoutReturningSecret(t *testing.T) {
	for _, failure := range []string{"response lost", "local commit", "cancelled", "late commit", "concurrent revoke"} {
		t.Run(failure, func(t *testing.T) {
			flow, repo, secrets := newRotation(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch failure {
			case "response lost":
				secrets.rotateErr = errors.New("response lost")
			case "local commit":
				repo.commitErr = errors.New("commit failed")
			case "cancelled":
				secrets.onRotate = func(context.Context) { cancel(); secrets.rotateErr = context.Canceled }
			case "late commit":
				secrets.onRotate = func(context.Context) { now := flow.now().Add(time.Minute); flow.now = func() time.Time { return now } }
			case "concurrent revoke":
				secrets.onRotate = func(context.Context) { repo.token.Status = domain.StatusRevoked }
			}
			result, err := flow.Rotate(ctx, repo.token)
			if err == nil || result.Secret != "" || repo.token.Status != domain.StatusRevoked || repo.operation.Status != domain.RotationAborted {
				t.Fatal("failed rotation leaked secret or remained active")
			}
			if !reflect.DeepEqual(secrets.calls, []string{"rotate", "revoke:old-key", "list:", "revoke:new-key"}) {
				t.Fatalf("unsafe cleanup sequence: %v", secrets.calls)
			}
		})
	}
}
func TestRotationRecoveryRetainsIntentAndResumesAfterRestart(t *testing.T) {
	flow, repo, secrets := newRotation(t)
	secrets.rotateErr, secrets.listErr = errors.New("lost response"), errors.New("list unavailable")
	if result, err := flow.Rotate(t.Context(), repo.token); err == nil || result.Secret != "" || repo.operation.Status != domain.RotationRecovering || repo.token.Status != domain.StatusRotating {
		t.Fatal("failed recovery discarded intent")
	}
	secrets.rotateErr, secrets.listErr, secrets.calls = nil, nil, nil
	restarted := NewAPITokenRotation(repo, secrets, rotationTx{})
	restarted.now = flow.now
	if _, err := restarted.ProcessBatch(t.Context(), ""); err != nil || repo.operation.Status != domain.RotationAborted {
		t.Fatal("restart did not complete pending cleanup")
	}
	for _, call := range secrets.calls {
		if call == "rotate" {
			t.Fatal("recovery blindly reissued rotation")
		}
	}
}
func TestRotationRecoveryWaitsForDeadlineAndCleansOnlyMatchingAttemptAcrossPages(t *testing.T) {
	flow, repo, secrets := newRotation(t)
	op := domain.Rotation{ID: "operation", TokenID: "token", OldKeyID: "old-key", ActorID: "User:owner", Status: domain.RotationPending, Deadline: flow.now().Add(time.Minute)}
	repo.operation, repo.token.Status = op, domain.StatusRotating
	if err := flow.Recover(t.Context(), op, false); err != nil || len(secrets.calls) != 0 {
		t.Fatal("live request was recovered before deadline")
	}
	secrets.pages = map[string]service.RotationKeyPage{
		"":     {Keys: []service.RotationKey{{KeyID: "unrelated", ActorID: op.ActorID, Metadata: map[string]any{"api_token_id": "other", "rotation_id": op.ID}}}, NextPageToken: "next"},
		"next": {Keys: []service.RotationKey{{KeyID: "replacement", ActorID: op.ActorID, Metadata: map[string]any{"api_token_id": op.TokenID, "rotation_id": op.ID}}, {KeyID: "previous-attempt", ActorID: op.ActorID, Metadata: map[string]any{"api_token_id": op.TokenID, "rotation_id": "previous"}}}},
	}
	if err := flow.Recover(t.Context(), op, true); err != nil || !reflect.DeepEqual(secrets.calls, []string{"revoke:old-key", "list:", "list:next", "revoke:replacement"}) {
		t.Fatalf("cross-attempt cleanup or lost page: %v %v", secrets.calls, err)
	}
}
func TestRotationRecoveryRejectsRepeatedPaginationCursor(t *testing.T) {
	flow, repo, secrets := newRotation(t)
	repo.operation = domain.Rotation{ID: "operation", TokenID: "token", ActorID: "User:owner", OldKeyID: "old-key", Status: domain.RotationPending}
	secrets.pages = map[string]service.RotationKeyPage{"": {NextPageToken: "repeat"}, "repeat": {NextPageToken: "repeat"}}
	if err := flow.Recover(t.Context(), repo.operation, true); !errors.Is(err, ErrRotationIncomplete) || repo.operation.Status != domain.RotationRecovering {
		t.Fatal("invalid pagination completed cleanup")
	}
}

func TestRotationDoesNotCallTalosWhenDurableIntentCannotCommit(t *testing.T) {
	flow, repo, secrets := newRotation(t)
	repo.beginErr = errors.New("database unavailable")
	result, err := flow.Rotate(t.Context(), repo.token)
	if err == nil || result.Secret != "" || len(secrets.calls) != 0 {
		t.Fatal("rotation escaped durable intent boundary")
	}
}

func TestRotationRecoveryMustFenceOldKeyBeforeEnumeratingReplacements(t *testing.T) {
	flow, repo, secrets := newRotation(t)
	secrets.rotateErr, secrets.revokeErr = errors.New("lost response"), errors.New("revoke unavailable")
	result, err := flow.Rotate(t.Context(), repo.token)
	if err == nil || result.Secret != "" || repo.operation.Status != domain.RotationRecovering || repo.token.Status != domain.StatusRotating {
		t.Fatal("unconfirmed old-key fence discarded cleanup intent")
	}
	if !reflect.DeepEqual(secrets.calls, []string{"rotate", "revoke:old-key", "get:old-key"}) {
		t.Fatal("enumerated keys while late rotation could still succeed")
	}
	secrets.calls = nil
	secrets.confirmedRevoke = true
	if _, err := flow.ProcessBatch(t.Context(), ""); err != nil || repo.operation.Status != domain.RotationAborted {
		t.Fatal("authoritative revoked query did not resolve lost revocation responses")
	}
}
