package workflow

import (
	"context"
	"log"
	"time"

	api "media_agent/media_auth/biz/domain/api_token"
	worker "media_agent/media_auth/biz/domain/worker_identity"
)

type WorkerIdentityLifecycle interface {
	GetOwned(context.Context, string, string) (worker.Identity, error)
	ChangeStatus(context.Context, string, string, worker.Status) error
}

type WorkerAccessRevocation struct {
	identities WorkerIdentityLifecycle
	source     worker.InactiveSource
	tokens     api.ActorRevocationRepository
	tx         TransactionRunner
}

func NewWorkerAccessRevocation(identities WorkerIdentityLifecycle, source worker.InactiveSource, tokens api.ActorRevocationRepository, tx TransactionRunner) *WorkerAccessRevocation {
	return &WorkerAccessRevocation{identities: identities, source: source, tokens: tokens, tx: tx}
}

func (w *WorkerAccessRevocation) ChangeStatus(ctx context.Context, owner, id string, next worker.Status) error {
	if next != worker.StatusDisabled && next != worker.StatusDeleted {
		return worker.ErrInvalid
	}
	return w.tx.RunInTx(ctx, func(txCtx context.Context) error {
		identity, err := w.identities.GetOwned(txCtx, owner, id)
		if err != nil {
			return err
		}
		if err := w.identities.ChangeStatus(txCtx, owner, id, next); err != nil {
			return err
		}
		return w.tokens.PrepareActorRevocation(txCtx, api.ActorPersonalWorker, identity.ActorID(), identity.OwnerUserID, identity.OrganizationID)
	})
}

// ProcessBatch catches issuance committed after a disable transaction's bulk
// update. Current actor validation already denies access; this sweep ensures
// the orphaned Talos key eventually reaches the existing recovery loops.
func (w *WorkerAccessRevocation) ProcessBatch(ctx context.Context, afterID string) (string, error) {
	identities, err := w.source.ListInactive(ctx, afterID, 100)
	if err != nil {
		return afterID, err
	}
	for _, identity := range identities {
		if err := ctx.Err(); err != nil {
			return afterID, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := w.tx.RunInTx(attemptCtx, func(txCtx context.Context) error {
			return w.tokens.PrepareActorRevocation(txCtx, api.ActorPersonalWorker, identity.ActorID(), identity.OwnerUserID, identity.OrganizationID)
		})
		cancel()
		if err != nil {
			log.Printf("worker access cleanup pending worker_id=%s", identity.ID)
		}
		afterID = identity.ID
	}
	if len(identities) < 100 {
		return "", nil
	}
	return afterID, nil
}

func (w *WorkerAccessRevocation) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		next, err := w.ProcessBatch(ctx, cursor)
		if err != nil && ctx.Err() == nil {
			log.Print("worker access cleanup scan failed")
		}
		cursor = next
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
