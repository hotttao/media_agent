//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"gorm.io/datatypes"
	"media_agent/media_workflow/biz/dal/model"
	jobrepo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
)

// An audit write must not revive an execution, clear its terminal metadata,
// or manufacture a new local retry epoch. Engine projection is separate.
func TestNativeRetryAuditDoesNotControlExecutionState(t *testing.T) {
	db := newDatabase(t)
	now := time.Now()
	if err := db.Create(&model.Job{ID: "retry-audit", Scope: "e2e", WorkflowID: "wf", WorkflowVersion: 1,
		Input: datatypes.JSON(`{}`), Status: "failed", FailureReason: "retained", RetryHistory: datatypes.JSON(`[]`),
		SubmittedBy: "test-owner", SubmittedAt: now, FinishedAt: &now, ConductorWorkflowID: "test-engine"}).Error; err != nil {
		t.Fatal(err)
	}
	repo := jobrepo.NewJobRepo(db)
	ctx := context.Background()
	if err := repo.AppendRetryHistory(ctx, "retry-audit", domain.RetryRecord{RetryAt: now, Reason: "continue"}, 0); err != nil {
		t.Fatal(err)
	}
	j, err := repo.Get(ctx, "retry-audit")
	if err != nil || j.Status != domain.JobFailed || j.FailureReason != "retained" || j.FinishedAt == nil || j.RetryCount != 0 || len(j.RetryHistory) != 1 {
		t.Fatal("retry audit changed engine projection or legacy epoch", err)
	}
}
