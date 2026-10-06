//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"media_agent/media_workflow/biz/dal"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	service "media_agent/media_workflow/biz/service/job"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/executable"
)

type retryCommitCompilation struct {
	wfdomain.CompilationRepo
	item *wfdomain.Compilation
}

func (c retryCommitCompilation) GetRegistered(context.Context, string, int) (*wfdomain.Compilation, error) {
	return c.item, nil
}

type retryCommitEngine struct {
	domain.WorkflowEngine
	db        *gorm.DB
	execution *domain.EngineExecution
	calls     int
	after     func()
}

func (e *retryCommitEngine) Get(context.Context, string) (*domain.EngineExecution, error) {
	return e.execution, nil
}
func (e *retryCommitEngine) RerunFromTask(ctx context.Context, _ string, _ string) error {
	if dal.FromContext(ctx, e.db) != e.db {
		return errors.New("engine call occurred inside local transaction")
	}
	e.calls++
	if e.after != nil {
		e.after()
	}
	return nil
}

func (e *retryCommitEngine) RerunWorkflow(ctx context.Context, _ string, _ map[string]any) error {
	return e.RerunFromTask(ctx, "", "")
}

type retryCommitFaultSteps struct {
	domain.StepRunRepo
	failID string
}

func (r retryCommitFaultSteps) UpdateStatus(ctx context.Context, id string, status domain.StepRunStatus, fields map[string]any) error {
	if err := r.StepRunRepo.UpdateStatus(ctx, id, status, fields); err != nil {
		return err
	}
	if id == r.failID {
		return errors.New("injected after actual step write")
	}
	return nil
}

type retryCommitFaultJob struct{ domain.JobRepo }

func (r retryCommitFaultJob) AppendRetryHistory(ctx context.Context, id string, record domain.RetryRecord, count int) error {
	if err := r.JobRepo.AppendRetryHistory(ctx, id, record, count); err != nil {
		return err
	}
	return errors.New("injected after actual history write")
}

// Real PostgreSQL atomicity and fencing, controlled engine acknowledgement.
// This does not prove upstream rerun durability or authorize replaying it.
func TestNativeStepRetryLocalAtomicCommit(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	jobs, steps := repo.NewJobRepo(db), repo.NewStepRunRepo(db)
	for _, scenario := range []string{"target_write", "downstream_write", "history_write", "cancelled", "epoch_changed", "success"} {
		t.Run(scenario, func(t *testing.T) {
			jobID := "retry_" + scenario
			j := &domain.Job{ID: jobID, WorkflowID: "frozen", WorkflowVersion: 1, Scope: "e2e", Origin: "native", Status: domain.JobFailed, SubmittedBy: "owner", SubmittedAt: time.Now()}
			runs := []*domain.StepRun{}
			for i, key := range []string{"root", "image", "middle", "sink"} {
				status := domain.SRSkipped
				if key == "root" {
					status = domain.SRSucceeded
				}
				if key == "image" {
					status = domain.SRFailed
				}
				runs = append(runs, &domain.StepRun{ID: jobID + "_" + key, JobID: jobID, StepKey: key, StepOrder: i + 1, TemplateID: "template", TemplateVersion: 1, Status: status, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates})
			}
			if jobs.Save(ctx, j, runs) != nil || jobs.UpdateConductorExecution(ctx, jobID, "engine_"+jobID, strings.Repeat("a", 64)) != nil {
				t.Fatal("isolated retry seed failed")
			}
			for _, run := range runs {
				if steps.UpdateStatus(ctx, run.ID, run.Status, map[string]any{"claimed_by": "prior-worker", "claimed_at": time.Now(), "started_at": time.Now(), "finished_at": time.Now()}) != nil {
					t.Fatal("retry timing seed failed")
				}
				if steps.UpdateCandidates(ctx, run.ID, []domain.Candidate{{URL: "object_path:retained"}}) != nil || steps.UpdateOutput(ctx, run.ID, "asset_"+run.StepKey) != nil {
					t.Fatal("candidate/output seed failed")
				}
			}
			ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: "frozen", WorkflowVersion: 1, OutputStepKey: "sink"}
			for i, key := range []string{"root", "image", "middle", "sink"} {
				step := executable.ExecutableStepIR{Key: key, Order: i + 1, Context: map[string]executable.ValueRef{}}
				if i > 0 {
					parent := []string{"root", "image", "middle"}[i-1]
					step.DependsOn = []string{parent}
					step.Context[parent] = executable.ValueRef{Source: executable.ValueStepOutput, Ref: parent}
				}
				ir.Steps = append(ir.Steps, step)
			}
			encoded, _ := json.Marshal(ir)
			comp := retryCommitCompilation{item: &wfdomain.Compilation{WorkflowID: "frozen", WorkflowVersion: 1, DefinitionHash: strings.Repeat("a", 64), IRJSON: encoded}}
			sequence := 1
			engine := &retryCommitEngine{db: db, execution: &domain.EngineExecution{Status: "FAILED", Tasks: []domain.EngineTask{{TaskID: "render", Sequence: &sequence, ReferenceTaskName: taskref.Reference("image", "render"), Status: "COMPLETED"}}}}
			var stepPort domain.StepRunRepo = steps
			var jobPort domain.JobRepo = jobs
			switch scenario {
			case "target_write":
				stepPort = retryCommitFaultSteps{StepRunRepo: steps, failID: runs[1].ID}
			case "downstream_write":
				stepPort = retryCommitFaultSteps{StepRunRepo: steps, failID: runs[2].ID}
			case "history_write":
				jobPort = retryCommitFaultJob{JobRepo: jobs}
			case "cancelled":
				engine.after = func() {
					if jobs.UpdateStatus(ctx, jobID, domain.JobCancelled, "") != nil {
						t.Error("cancellation injection failed")
					}
				}
			case "epoch_changed":
				engine.after = func() {
					if jobs.AppendRetryHistory(ctx, jobID, domain.RetryRecord{Reason: "concurrent retry"}, 1) != nil {
						t.Error("epoch injection failed")
					}
				}
			}
			svc := service.NewService(jobPort, stepPort, dal.NewTxRunner(db), nil, nil, nil, nil, comp, engine)
			svc.ConfigureReviews(jobs)
			_, err := svc.RetryStepRun(ctx, runs[1].ID, "isolated retry", nil)
			if (err == nil) != (scenario == "success") || engine.calls != 1 {
				t.Fatal("unexpected retry outcome")
			}
			current, _ := jobs.Get(ctx, jobID)
			if scenario == "success" {
				if current.Status != domain.JobRunning || current.RetryCount != 1 || len(current.RetryHistory) != 1 {
					t.Fatal("job history/count not atomically committed")
				}
			} else if scenario == "cancelled" {
				if current.Status != domain.JobCancelled || current.RetryCount != 0 {
					t.Fatal("cancelled job revived")
				}
			} else if scenario == "epoch_changed" {
				if current.RetryCount != 1 || len(current.RetryHistory) != 1 || current.RetryHistory[0].Reason != "concurrent retry" {
					t.Fatal("late commit overwrote newer epoch")
				}
			} else if current.Status != domain.JobFailed || current.RetryCount != 0 || len(current.RetryHistory) != 0 {
				t.Fatal("partial retry job state escaped rollback")
			}
			for i, run := range runs {
				saved, _ := steps.Get(ctx, run.ID)
				if scenario == "success" && i > 0 {
					if saved.Status != domain.SRBlocked || len(saved.Candidates) != 0 || saved.OutputAssetID != "" || saved.ClaimedBy != "" || saved.ClaimedAt != nil || saved.StartedAt != nil || saved.FinishedAt != nil {
						t.Fatal("affected step did not commit reset")
					}
					if i == 1 && saved.RetryCount != 1 {
						t.Fatal("target retry count not committed")
					}
				} else if saved.Status != run.Status || len(saved.Candidates) != 1 || saved.OutputAssetID != "asset_"+run.StepKey || saved.RetryCount != 0 || saved.ClaimedBy != "prior-worker" || saved.FinishedAt == nil {
					t.Fatal("partial or collateral step write escaped transaction")
				}
			}
		})
	}
}

func TestNativeJobRetryLocalAtomicCommit(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	jobs, steps := repo.NewJobRepo(db), repo.NewStepRunRepo(db)
	for _, scenario := range []string{"step_write", "history_write", "cancelled", "epoch_changed", "success"} {
		t.Run(scenario, func(t *testing.T) {
			id := "whole_retry_" + scenario
			job := &domain.Job{ID: id, WorkflowID: "workflow", WorkflowVersion: 1, Scope: "e2e", Origin: "native", Status: domain.JobFailed, SubmittedBy: "owner", SubmittedAt: time.Now()}
			runs := []*domain.StepRun{
				{ID: id + "_one", JobID: id, StepKey: "one", StepOrder: 1, TemplateID: "template", TemplateVersion: 1, Status: domain.SRSucceeded, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates},
				{ID: id + "_two", JobID: id, StepKey: "two", StepOrder: 2, TemplateID: "template", TemplateVersion: 1, Status: domain.SRFailed, ApprovalPolicy: domain.ApprovalWhenMultipleCandidates},
			}
			if jobs.Save(ctx, job, runs) != nil || jobs.UpdateConductorExecution(ctx, id, "engine_"+id, strings.Repeat("b", 64)) != nil {
				t.Fatal("whole retry seed failed")
			}
			for _, run := range runs {
				if steps.UpdateStatus(ctx, run.ID, run.Status, map[string]any{"claimed_by": "prior-worker", "claimed_at": time.Now(), "started_at": time.Now(), "finished_at": time.Now()}) != nil {
					t.Fatal("whole retry timing seed failed")
				}
				if steps.UpdateCandidates(ctx, run.ID, []domain.Candidate{{URL: "object_path:prior"}}) != nil || steps.UpdateOutput(ctx, run.ID, "asset_"+run.StepKey) != nil {
					t.Fatal("whole retry output seed failed")
				}
			}
			var stepPort domain.StepRunRepo = steps
			var jobPort domain.JobRepo = jobs
			engine := &retryCommitEngine{db: db}
			switch scenario {
			case "step_write":
				stepPort = retryCommitFaultSteps{StepRunRepo: steps, failID: runs[1].ID}
			case "history_write":
				jobPort = retryCommitFaultJob{JobRepo: jobs}
			case "cancelled":
				engine.after = func() {
					if jobs.UpdateStatus(ctx, id, domain.JobCancelled, "") != nil {
						t.Error("cancel injection failed")
					}
				}
			case "epoch_changed":
				engine.after = func() {
					if jobs.AppendRetryHistory(ctx, id, domain.RetryRecord{Reason: "concurrent"}, 1) != nil {
						t.Error("epoch injection failed")
					}
				}
			}
			svc := service.NewService(jobPort, stepPort, dal.NewTxRunner(db), nil, nil, nil, nil, nil, engine)
			svc.ConfigureReviews(jobs)
			_, err := svc.Retry(ctx, id, "whole retry", nil)
			if (err == nil) != (scenario == "success") || engine.calls != 1 {
				t.Fatal("wrong whole retry result")
			}
			current, _ := jobs.Get(ctx, id)
			switch scenario {
			case "success":
				if current.Status != domain.JobRunning || current.RetryCount != 1 || len(current.RetryHistory) != 1 {
					t.Fatal("whole retry facts not committed")
				}
			case "cancelled":
				if current.Status != domain.JobCancelled || current.RetryCount != 0 {
					t.Fatal("whole retry revived cancelled job")
				}
			case "epoch_changed":
				if current.RetryCount != 1 || len(current.RetryHistory) != 1 || current.RetryHistory[0].Reason != "concurrent" {
					t.Fatal("whole retry overwrote new epoch")
				}
			default:
				if current.Status != domain.JobFailed || current.RetryCount != 0 || len(current.RetryHistory) != 0 {
					t.Fatal("whole retry partial job commit")
				}
			}
			for _, run := range runs {
				saved, _ := steps.Get(ctx, run.ID)
				if scenario == "success" {
					if saved.Status != domain.SRBlocked || len(saved.Candidates) != 0 || saved.OutputAssetID != "" || saved.ClaimedBy != "" || saved.ClaimedAt != nil || saved.StartedAt != nil || saved.FinishedAt != nil {
						t.Fatal("whole retry reset missing")
					}
				} else if saved.Status != run.Status || len(saved.Candidates) != 1 || saved.OutputAssetID != "asset_"+run.StepKey || saved.ClaimedBy != "prior-worker" || saved.FinishedAt == nil {
					t.Fatal("whole retry step did not roll back")
				}
			}
		})
	}
}
