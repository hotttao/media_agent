package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	api "media_agent/media_auth/biz/domain/api_token"
	worker "media_agent/media_auth/biz/domain/worker_identity"
	service "media_agent/media_auth/biz/service/api_token"
)

type workerTokenCreator struct {
	command service.CreateCommand
	calls   int
	err     error
}

func (c *workerTokenCreator) Create(_ context.Context, cmd service.CreateCommand) (service.CreateResult, error) {
	c.command = cmd
	c.calls++
	return service.CreateResult{}, c.err
}

func TestWorkerTokenProvisioningDerivesAuthorityAndRestrictsScopes(t *testing.T) {
	identity := &workerLifecycle{identity: worker.Identity{ID: "stable", OwnerUserID: "owner", OrganizationID: "org", Status: worker.StatusActive}}
	tokens := &workerTokenCreator{}
	flow := NewWorkerTokenProvisioning(identity, tokens)
	cmd := CreateWorkerTokenCommand{OwnerUserID: "owner", WorkerID: "stable", Name: "desktop", ExpiresAt: time.Now().Add(time.Hour), IdempotencyKey: "once"}
	if _, err := flow.Create(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	got := tokens.command
	if got.CreatorUserID != "owner" || got.SubjectUserID != "owner" || got.ActorType != api.ActorPersonalWorker || got.ActorID != "Worker:stable" || got.OrganizationID != "org" || !got.TrustedActorCreation || len(got.Scopes) != 5 || got.IdempotencyKey != "once" {
		t.Fatal("worker token command did not preserve derived identity")
	}
	cmd.Scopes = []string{" workflow.worker.poll "}
	if _, err := flow.Create(t.Context(), cmd); err != nil || len(tokens.command.Scopes) != 1 || tokens.command.Scopes[0] != "workflow.worker.poll" {
		t.Fatal("subset not preserved")
	}
	for _, scopes := range [][]string{{"api.access"}, {"agent.gateway"}, {"workflow.worker.poll", "api.access"}, {""}, {"workflow.worker.register"}} {
		cmd.Scopes = scopes
		before := tokens.calls
		if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, service.ErrInvalidRequest) || before != tokens.calls {
			t.Fatal("scope escalation reached issuer")
		}
	}
	cmd.Scopes = nil
	cmd.OwnerUserID = "other"
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, worker.ErrNotFound) {
		t.Fatal("other owner accepted")
	}
	cmd.OwnerUserID = "owner"
	identity.identity.Status = worker.StatusDisabled
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, worker.ErrForbidden) {
		t.Fatal("disabled actor accepted")
	}
	identity.identity.Status = worker.StatusActive
	tokens.err = api.ErrConflict
	if _, err := flow.Create(t.Context(), cmd); !errors.Is(err, api.ErrConflict) {
		t.Fatal("persistent issuance conflict lost")
	}
}
