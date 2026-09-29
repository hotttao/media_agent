package workflow

import (
	"context"
	"strings"
	"time"

	api "media_agent/media_auth/biz/domain/api_token"
	worker "media_agent/media_auth/biz/domain/worker_identity"
	service "media_agent/media_auth/biz/service/api_token"
)

type WorkerIdentityReader interface {
	GetOwned(context.Context, string, string) (worker.Identity, error)
}
type WorkerTokenCreator interface {
	Create(context.Context, service.CreateCommand) (service.CreateResult, error)
}
type WorkerTokenProvisioning struct {
	identities WorkerIdentityReader
	tokens     WorkerTokenCreator
}

func NewWorkerTokenProvisioning(identities WorkerIdentityReader, tokens WorkerTokenCreator) *WorkerTokenProvisioning {
	return &WorkerTokenProvisioning{identities: identities, tokens: tokens}
}

type CreateWorkerTokenCommand struct {
	OwnerUserID, WorkerID, Name, IdempotencyKey string
	Scopes                                      []string
	ExpiresAt                                   time.Time
}

var workerScopes = []string{"workflow.worker.poll", "workflow.worker.heartbeat", "workflow.worker.complete", "workflow.worker.fail", "workflow.worker.upload"}

func (w *WorkerTokenProvisioning) Create(ctx context.Context, cmd CreateWorkerTokenCommand) (service.CreateResult, error) {
	identity, err := w.identities.GetOwned(ctx, cmd.OwnerUserID, cmd.WorkerID)
	if err != nil {
		return service.CreateResult{}, err
	}
	if identity.Status != worker.StatusActive {
		return service.CreateResult{}, worker.ErrForbidden
	}
	scopes := append([]string(nil), cmd.Scopes...)
	if len(scopes) == 0 {
		scopes = append(scopes, workerScopes...)
	}
	for i, scope := range scopes {
		scope = strings.TrimSpace(scope)
		allowed := false
		for _, valid := range workerScopes {
			if scope == valid {
				allowed = true
				break
			}
		}
		if !allowed {
			return service.CreateResult{}, service.ErrInvalidRequest
		}
		scopes[i] = scope
	}
	return w.tokens.Create(ctx, service.CreateCommand{CreatorUserID: cmd.OwnerUserID, SubjectUserID: identity.OwnerUserID,
		ActorType: api.ActorPersonalWorker, ActorID: identity.ActorID(), OrganizationID: identity.OrganizationID,
		Name: cmd.Name, Scopes: scopes, ExpiresAt: cmd.ExpiresAt, IdempotencyKey: cmd.IdempotencyKey, TrustedActorCreation: true})
}
