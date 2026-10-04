//go:build e2e

package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/datatypes"

	"media_agent/media_workflow/biz/dal/model"
	jobrepo "media_agent/media_workflow/biz/dal/repo/job"
	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/workflow/worker_gateway"
)

// Real isolated PostgreSQL; no live users, jobs, tokens or queues are touched.
// This verifies the business authorization boundary, not gateway JWT exchange.
func TestPersonalDispatchAuthorizationIsolation(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	if err := db.Create(&model.Job{ID: "job-auth", OrganizationID: "org-a", Scope: "e2e", WorkflowID: "wf-auth", WorkflowVersion: 1,
		Input: datatypes.JSON(`{}`), Status: "running", RetryHistory: datatypes.JSON(`[]`), SubmittedBy: "owner", SubmittedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.StepRun{ID: "step-auth", JobID: "job-auth", StepKey: "image", StepOrder: 1,
		TemplateID: "tpl-auth", TemplateVersion: 1, Status: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	repo := jobrepo.NewDispatchRepo(db)
	_, err := repo.Create(ctx, &jobdomain.WorkerDispatch{ID: "dispatch-auth", JobID: "job-auth", StepRunID: "step-auth",
		ConductorTaskID: "task-auth", ConductorWorkflowID: "engine-auth", Attempt: 1, CapabilityKey: "image", CapabilityVersion: 1,
		UserID: "owner", OrganizationID: "org-a", ActorID: "Worker:one", CredentialID: "credential-one", AuthSource: "api_token",
		WorkerInstanceID: "instance-one", Status: jobdomain.DispatchDelivered, InputJSON: []byte(`{}`), LeaseExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	before, err := repo.Get(ctx, "dispatch-auth")
	if err != nil {
		t.Fatal(err)
	}
	engine := &resultEngine{}
	flow := &worker_gateway.Flow{Dispatches: repo, Jobs: jobrepo.NewJobRepo(db), Engine: engine}
	owner := worker_gateway.Principal{UserID: "owner", OrganizationID: "org-a", ActorID: "Worker:one", CredentialID: "credential-one", AuthSource: "api_token"}
	output := map[string]any{"candidates": []any{map[string]any{"content": "test-only"}}}
	cases := []struct {
		name   string
		mutate func(*worker_gateway.Principal, *string, *int)
		want   error
	}{
		{"user", func(p *worker_gateway.Principal, _ *string, _ *int) { p.UserID = "other" }, worker_gateway.ErrForbidden},
		{"organization", func(p *worker_gateway.Principal, _ *string, _ *int) { p.OrganizationID = "org-b" }, worker_gateway.ErrForbidden},
		{"actor", func(p *worker_gateway.Principal, _ *string, _ *int) { p.ActorID = "Worker:two" }, worker_gateway.ErrForbidden},
		{"credential", func(p *worker_gateway.Principal, _ *string, _ *int) { p.CredentialID = "credential-two" }, worker_gateway.ErrForbidden},
		{"instance", func(_ *worker_gateway.Principal, instance *string, _ *int) { *instance = "instance-two" }, worker_gateway.ErrForbidden},
		{"auth_source", func(p *worker_gateway.Principal, _ *string, _ *int) { p.AuthSource = "internal_runtime" }, worker_gateway.ErrUnauthorized},
		{"attempt", func(_ *worker_gateway.Principal, _ *string, attempt *int) { *attempt = 2 }, jobdomain.ErrDispatchConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			principal, instance, attempt := owner, "instance-one", 1
			tc.mutate(&principal, &instance, &attempt)
			_, heartbeatErr := flow.Heartbeat(ctx, principal, "dispatch-auth", instance, attempt)
			_, completeErr := flow.Complete(ctx, worker_gateway.CompleteCommand{Principal: principal, DispatchID: "dispatch-auth", WorkerInstanceID: instance,
				Attempt: attempt, IdempotencyKey: "complete-" + tc.name, Output: output})
			_, failErr := flow.Fail(ctx, worker_gateway.FailCommand{Principal: principal, DispatchID: "dispatch-auth", WorkerInstanceID: instance,
				Attempt: attempt, IdempotencyKey: "fail-" + tc.name, Reason: "test-only"})
			for _, got := range []error{heartbeatErr, completeErr, failErr} {
				if !errors.Is(got, tc.want) {
					t.Fatalf("operation error=%v want=%v", got, tc.want)
				}
			}
		})
	}
	var count int64
	if err := db.Model(&model.WorkerResultInbox{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rejected operations wrote a result")
	}
	after, err := repo.Get(ctx, "dispatch-auth")
	if err != nil || after.Status != before.Status || !after.LeaseExpiresAt.Equal(before.LeaseExpiresAt) || engine.result.TaskID != "" {
		t.Fatal("rejected operations changed lease, status or Conductor")
	}
	command := worker_gateway.CompleteCommand{Principal: owner, DispatchID: "dispatch-auth", WorkerInstanceID: "instance-one",
		Attempt: 1, IdempotencyKey: "valid-complete", Output: output}
	first, err := flow.Complete(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := flow.Complete(ctx, command)
	if err != nil || first.ID != replay.ID {
		t.Fatal("authorized result replay was not idempotent")
	}
	if err := db.Model(&model.WorkerResultInbox{}).Count(&count).Error; err != nil || count != 1 || engine.result.TaskID != "" {
		t.Fatal("authorized result must be persisted once for the durable submitter")
	}
}
