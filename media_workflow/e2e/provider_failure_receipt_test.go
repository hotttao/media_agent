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

// Real isolated PostgreSQL; the Conductor transport is an explicit port stub.
func TestProviderFailureDirectProxyPersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	for _, kind := range []string{"", "permanent", "transient"} {
		t.Run("kind="+kind, func(t *testing.T) {
			id := "failure-" + kind
			if err := db.Create(&model.Job{ID: id, OrganizationID: "org-a", Scope: "e2e", WorkflowID: "wf", WorkflowVersion: 1,
				Input: datatypes.JSON(`{}`), Status: "failed", RetryHistory: datatypes.JSON(`[]`), SubmittedBy: "owner", SubmittedAt: time.Now(), ConductorWorkflowID: id}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.StepRun{ID: id, JobID: id, StepKey: "image", StepOrder: 1, TemplateID: "tpl", TemplateVersion: 1, Status: "failed"}).Error; err != nil {
				t.Fatal(err)
			}
			repo := jobrepo.NewDispatchRepo(db)
			if _, err := repo.Create(ctx, &jobdomain.WorkerDispatch{ID: id, JobID: id, StepRunID: id, ConductorTaskID: id, ConductorWorkflowID: id,
				Attempt: 1, CapabilityKey: "image", CapabilityVersion: 1, UserID: "owner", OrganizationID: "org-a", ActorID: "Worker:one",
				CredentialID: "credential", AuthSource: "api_token", WorkerInstanceID: "desktop", Status: jobdomain.DispatchDelivered, InputJSON: []byte(`{}`), LeaseExpiresAt: time.Now().Add(time.Minute)}); err != nil {
				t.Fatal(err)
			}
			engine := &resultEngine{execution: &jobdomain.EngineExecution{WorkflowInstanceID: id, Status: "RUNNING", Tasks: []jobdomain.EngineTask{{TaskID: id, Status: "IN_PROGRESS", Input: map[string]any{"job_id": id}}}}}
			flow := &worker_gateway.Flow{Dispatches: repo, Jobs: jobrepo.NewJobRepo(db), Engine: engine, Executions: engine}
			cmd := worker_gateway.FailCommand{Principal: worker_gateway.Principal{UserID: "owner", OrganizationID: "org-a", ActorID: "Worker:one", CredentialID: "credential", AuthSource: "api_token"},
				DispatchID: id, WorkerInstanceID: "desktop", Attempt: 1, IdempotencyKey: "stable", Reason: "provider unavailable", FailureKind: kind}
			invalid := cmd
			invalid.FailureKind = "unknown"
			if _, err := flow.Fail(ctx, invalid); !errors.Is(err, worker_gateway.ErrInvalidRequest) {
				t.Fatal("unknown classification accepted")
			}
			first, err := flow.Fail(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := flow.Fail(ctx, cmd)
			if err != nil || replay.ID != first.ID {
				t.Fatal("same failure was not replayable")
			}
			if kind == "" {
				explicit := cmd
				explicit.FailureKind = "permanent"
				if receipt, err := flow.Fail(ctx, explicit); err != nil || receipt.ID != first.ID {
					t.Fatal("explicit permanent changed legacy receipt")
				}
			}
			changed := cmd
			changed.FailureKind = "transient"
			if kind == "transient" {
				changed.FailureKind = "permanent"
			}
			if _, err := flow.Fail(ctx, changed); !errors.Is(err, jobdomain.ErrDispatchConflict) {
				t.Fatal("changed classification overwrote failure")
			}
			var count int64
			if err := db.Model(&model.WorkerResultInbox{}).Where("dispatch_id = ?", id).Count(&count).Error; err != nil || count != 0 || first.ConductorStatus != "submitted" {
				t.Fatal("direct proxy must not create an inbox receipt")
			}
			got, err := repo.Get(ctx, id)
			if err != nil || got.Status != jobdomain.DispatchResultReceived || got.CompletedAt == nil {
				t.Fatal("dispatch acceptance did not commit with receipt")
			}
			want := "FAILED_WITH_TERMINAL_ERROR"
			if kind == "transient" {
				want = "FAILED"
			}
			if engine.result.TaskID != id || engine.result.Status != want || engine.result.Output != nil {
				t.Fatal("persisted classification was not delivered")
			}
			var job model.Job
			if err := db.First(&job, "id = ?", id).Error; err != nil || job.Status != "failed" || job.RetryCount != 0 {
				t.Fatal("receipt handling reset local execution")
			}
		})
	}
}
