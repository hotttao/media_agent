//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"

	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	service "media_agent/media_workflow/biz/service/job"
)

// Real PostgreSQL projection and downgrade safety; controlled engine snapshot.
func TestPausedProjectionPersistenceAndDowngradeSafety(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	jobs, steps := repo.NewJobRepo(db), repo.NewStepRunRepo(db)
	j := &domain.Job{ID: "job-pause", WorkflowID: "workflow", WorkflowVersion: 1, Scope: "e2e", SubmittedBy: "owner", Status: domain.JobRunning, ConductorWorkflowID: "execution"}
	sr := &domain.StepRun{ID: "step-pause", JobID: j.ID, StepKey: "image", TemplateID: "template", TemplateVersion: 1, Status: domain.SRPending}
	if err := jobs.Save(ctx, j, []*domain.StepRun{sr}); err != nil {
		t.Fatal("isolated seed failed")
	}
	// Save creates a not-yet-bound aggregate; binding is a separate port.
	if err := jobs.UpdateConductorExecution(ctx, j.ID, "execution", "frozen"); err != nil {
		t.Fatal("isolated engine binding failed")
	}
	engine := reviewProjectionEngine{execution: &domain.EngineExecution{WorkflowInstanceID: "execution", Status: "PAUSED"}}
	svc := service.NewService(jobs, steps, dal.NewTxRunner(db), nil, nil, nil, nil, nil, engine)
	got, _, err := svc.Get(ctx, j.ID)
	if err != nil || got.Status != domain.JobPaused || got.FinishedAt != nil {
		t.Fatal("pause was not persisted as a non-terminal projection", err)
	}

	down, err := os.ReadFile("../migrations/000024_job_paused_projection.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, string(down)); err == nil {
		t.Fatal("downgrade silently reinterpreted active pause")
	}
	// The failed multi-statement migration leaves its connection aborted.
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatal("isolated rollback failed")
	}
	got, err = jobs.Get(ctx, j.ID)
	if err != nil || got.Status != domain.JobPaused {
		t.Fatal("failed downgrade changed pause")
	}
	engine.execution.Status = "RUNNING"
	got, _, err = svc.Get(ctx, j.ID)
	if err != nil || got.Status != domain.JobRunning {
		t.Fatal("native resume did not refresh projection", err)
	}
	engine.execution.Status = "TERMINATED"
	got, _, err = svc.Get(ctx, j.ID)
	if err != nil || got.Status != domain.JobCancelled {
		t.Fatal("native termination was reported as failure", err)
	}
	if _, err := conn.ExecContext(ctx, string(down)); err != nil {
		t.Fatal("safe downgrade failed without paused rows")
	}
}
