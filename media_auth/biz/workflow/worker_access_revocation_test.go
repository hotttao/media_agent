package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	api "media_agent/media_auth/biz/domain/api_token"
	worker "media_agent/media_auth/biz/domain/worker_identity"
)

type workerLifecycle struct {
	identity worker.Identity
	err      error
	changed  bool
}

func (l *workerLifecycle) GetOwned(_ context.Context, owner, id string) (worker.Identity, error) {
	if l.err != nil {
		return worker.Identity{}, l.err
	}
	if owner != l.identity.OwnerUserID || id != l.identity.ID {
		return worker.Identity{}, worker.ErrNotFound
	}
	return l.identity, nil
}
func (l *workerLifecycle) ChangeStatus(_ context.Context, _, _ string, next worker.Status) error {
	l.changed = true
	l.identity.Status = next
	return l.err
}

type workerInactive struct {
	rows []worker.Identity
	err  error
}

func (s *workerInactive) ListInactive(_ context.Context, after string, limit int) ([]worker.Identity, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := []worker.Identity{}
	for _, row := range s.rows {
		if row.ID > after {
			out = append(out, row)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

type actorCleanup struct {
	calls     []worker.Identity
	failActor string
}

func (r *actorCleanup) PrepareActorRevocation(_ context.Context, kind api.ActorType, actor, owner, org string) error {
	if kind != api.ActorPersonalWorker {
		return errors.New("wrong actor type")
	}
	r.calls = append(r.calls, worker.Identity{ID: actor, OwnerUserID: owner, OrganizationID: org})
	if actor == r.failActor {
		return errors.New("database offline")
	}
	return nil
}

func TestWorkerAccessRevocationAuthorizesOwnerAndStoresExactActorIntent(t *testing.T) {
	identity := &workerLifecycle{identity: worker.Identity{ID: "stable", OwnerUserID: "owner", OrganizationID: "org", Status: worker.StatusActive}}
	tokens := &actorCleanup{}
	flow := NewWorkerAccessRevocation(identity, &workerInactive{}, tokens, rotationTx{})
	if err := flow.ChangeStatus(t.Context(), "other", "stable", worker.StatusDisabled); !errors.Is(err, worker.ErrNotFound) || identity.changed || len(tokens.calls) != 0 {
		t.Fatal("other owner revoked identity")
	}
	if err := flow.ChangeStatus(t.Context(), "owner", "stable", worker.StatusActive); !errors.Is(err, worker.ErrInvalid) || identity.changed {
		t.Fatal("activation accepted")
	}
	if err := flow.ChangeStatus(t.Context(), "owner", "stable", worker.StatusDisabled); err != nil || !identity.changed || len(tokens.calls) != 1 {
		t.Fatal(err)
	}
	if got := tokens.calls[0]; got.ID != "Worker:stable" || got.OwnerUserID != "owner" || got.OrganizationID != "org" {
		t.Fatal("cleanup lost actor boundary")
	}
	tokens.failActor = "Worker:stable"
	if err := flow.ChangeStatus(t.Context(), "owner", "stable", worker.StatusDisabled); err == nil {
		t.Fatal("cleanup failure hidden")
	}
}

func TestWorkerAccessRevocationScanPagesPastFailuresAndRetries(t *testing.T) {
	source := &workerInactive{}
	for i := 0; i < 101; i++ {
		source.rows = append(source.rows, worker.Identity{ID: fmt.Sprintf("worker-%03d", i), OwnerUserID: "owner", OrganizationID: "org", Status: worker.StatusDisabled})
	}
	tokens := &actorCleanup{failActor: "Worker:worker-000"}
	flow := NewWorkerAccessRevocation(&workerLifecycle{}, source, tokens, rotationTx{})
	cursor, err := flow.ProcessBatch(t.Context(), "")
	if err != nil || cursor != "worker-099" || len(tokens.calls) != 100 {
		t.Fatal("failure starved page", err)
	}
	cursor, err = flow.ProcessBatch(t.Context(), cursor)
	if err != nil || cursor != "" || len(tokens.calls) != 101 {
		t.Fatal("failure starved later page", err)
	}
	tokens.calls = nil
	if _, err := flow.ProcessBatch(t.Context(), ""); err != nil || tokens.calls[0].ID != "Worker:worker-000" {
		t.Fatal("failed row not retried")
	}
	source.err = errors.New("scan unavailable")
	if cursor, err := flow.ProcessBatch(t.Context(), "last"); !errors.Is(err, source.err) || cursor != "last" {
		t.Fatal("scan failure lost cursor")
	}
	source.err = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tokens.calls = nil
	if _, err := flow.ProcessBatch(ctx, ""); !errors.Is(err, context.Canceled) || len(tokens.calls) != 0 {
		t.Fatal("canceled sweep changed records")
	}
	flow.Run(ctx)
}
