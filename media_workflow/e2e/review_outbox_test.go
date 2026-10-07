//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
	jobservice "media_agent/media_workflow/biz/service/job"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/review_delivery"
)

type committedReviewResponseLoss struct {
	domain.WorkflowEngine
	calls int
}

type reviewProcessCompletion struct {
	domain.WorkflowEngine
	crashAfterCommit bool
	forbidCompletion bool
}

func (e reviewProcessCompletion) CompleteApprovalTask(ctx context.Context, id, taskID string, output map[string]any) error {
	if e.forbidCompletion {
		return errors.New("recovered committed review must not be resubmitted")
	}
	if err := e.WorkflowEngine.(domain.ApprovalTaskEngine).CompleteApprovalTask(ctx, id, taskID, output); err != nil {
		return err
	}
	if e.crashAfterCommit {
		os.Exit(77)
	}
	return nil
}

type reviewProjectionEngine struct {
	domain.WorkflowEngine
	execution *domain.EngineExecution
}

func (e reviewProjectionEngine) Get(context.Context, string) (*domain.EngineExecution, error) {
	return e.execution, nil
}

type reviewWorkspaceAccess struct{}

func (reviewWorkspaceAccess) CanUseWorkflow(context.Context, string, string) (bool, error) {
	return true, nil
}

func (e *committedReviewResponseLoss) CompleteApprovalTask(ctx context.Context, workflowID, taskID string, output map[string]any) error {
	e.calls++
	if err := e.WorkflowEngine.(domain.ApprovalTaskEngine).CompleteApprovalTask(ctx, workflowID, taskID, output); err != nil {
		return err
	}
	return errors.New("injected response loss after Conductor commit")
}

// The upstream and database are real; only the transport response is dropped.
// This proves recovery of an accepted internal command, not user API wiring.
func TestReviewDeliveryRealConductorLostResponse(t *testing.T) {
	testReviewDeliveryRecovery(t, "")
}

func TestReviewDeliveryRealProcessRestart(t *testing.T) {
	for _, mode := range []string{"claim-and-exit", "commit-and-exit"} {
		t.Run(mode, func(t *testing.T) { testReviewDeliveryRecovery(t, mode) })
	}
}

// This subprocess uses only the parent's isolated database. Exit after the
// persisted claim deliberately bypasses defers and leaves the lease pending.
func TestReviewDeliveryProcessHelper(t *testing.T) {
	mode := os.Getenv("MEDIA_E2E_REVIEW_CHILD_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	db, err := gorm.Open(pgdriver.Open(os.Getenv("MEDIA_E2E_REVIEW_CHILD_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("isolated child database unavailable")
	}
	store := repo.NewJobRepo(db)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	switch mode {
	case "claim-and-exit":
		review, err := store.ClaimPendingReview(ctx)
		if err != nil || review == nil || review.ID != os.Getenv("MEDIA_E2E_REVIEW_CHILD_ID") {
			t.Fatal("child did not persist the expected claim")
		}
		os.Exit(77)
	case "deliver", "commit-and-exit":
		engine := reviewProcessCompletion{WorkflowEngine: &conductor.ExecutionEngine{Client: conductor.NewClient(os.Getenv("MEDIA_E2E_CONDUCTOR_URL"))},
			crashAfterCommit: mode == "commit-and-exit", forbidCompletion: os.Getenv("MEDIA_E2E_REVIEW_CHILD_ALREADY_COMMITTED") == "true" && mode == "deliver"}
		flow := &review_delivery.Flow{Reviews: store, Jobs: store, Engine: engine}
		if err := flow.DispatchOne(ctx); err != nil {
			t.Fatal("new process did not deliver pending review")
		}
	default:
		t.Fatal("unknown child mode")
	}
}

func testReviewDeliveryRecovery(t *testing.T, crashMode string) {
	address := os.Getenv("MEDIA_E2E_CONDUCTOR_URL")
	if address == "" {
		t.Skip("set MEDIA_E2E_CONDUCTOR_URL")
	}
	db := newDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := "review_delivery_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	api := conductor.NewClient(address)
	candidates := []domain.Candidate{{Content: "isolated_review_candidate"}}
	definition := compiler.Definition{Name: name, Version: 1, SchemaVersion: 2, Tasks: []compiler.Task{{
		Name: "review", TaskReferenceName: "review", Type: "HUMAN", InputParameters: map[string]any{"candidates": candidates},
	}}}
	if err := api.RegisterWorkflow(ctx, definition); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, address+"/api/metadata/workflow/"+name+"/1", nil)
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Error("isolated review definition cleanup failed")
			return
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Errorf("review cleanup HTTP %d", resp.StatusCode)
		}
	})
	id, err := api.StartWorkflow(ctx, conductor.StartWorkflowRequest{Name: name, Version: 1, Input: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		execution, err := api.GetWorkflow(context.Background(), id)
		if err == nil && execution.Status == "RUNNING" {
			_ = api.TerminateWorkflow(context.Background(), id, "isolated review delivery cleanup")
		}
	})
	var taskID string
	for taskID == "" {
		execution, err := api.GetWorkflow(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range execution.Tasks {
			if task.ReferenceTaskName == "review" && task.Status == "IN_PROGRESS" {
				taskID = task.TaskID
			}
		}
		if taskID == "" {
			select {
			case <-ctx.Done():
				t.Fatal("isolated HUMAN did not start")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	job := &model.Job{ID: "job_" + name, Scope: "e2e", WorkflowID: name, WorkflowVersion: 1, Input: datatypes.JSON(`{}`),
		Status: "need_confirm", RetryHistory: datatypes.JSON(`[]`), SubmittedBy: "isolated-owner", SubmittedAt: time.Now(), ConductorWorkflowID: id}
	if err := db.Create(job).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(candidates)
	step := &model.StepRun{ID: "step_" + name, JobID: job.ID, StepKey: "image", StepOrder: 1, TemplateID: "e2e", TemplateVersion: 1,
		Status: "need_confirm", Candidates: datatypes.JSON(snapshot)}
	if err := db.Create(step).Error; err != nil {
		t.Fatal(err)
	}
	index := 0
	output, _ := json.Marshal(map[string]any{"decision": "accept", "selected_index": 0, "selected_candidate": candidates[0]})
	review := domain.ReviewDecision{ID: "review_" + name, JobID: job.ID, StepRunID: step.ID, ConductorWorkflowID: id, ConductorTaskID: taskID,
		TaskReferenceName: "review", Generation: 0, Decision: "accept", DecidedBy: job.SubmittedBy, IdempotencyKey: "once", SelectedIndex: &index,
		CandidatesSnapshot: snapshot, OutputJSON: output}
	review.RequestDigest, _ = review.Digest()
	store := repo.NewJobRepo(db)
	if err := dal.NewTxRunner(db).RunInTx(ctx, func(txCtx context.Context) error { _, _, err := store.AcceptReview(txCtx, review); return err }); err != nil {
		t.Fatal(err)
	}
	if crashMode != "" {
		runChild := func(mode string, expectedExit int) {
			t.Helper()
			binary, err := os.Executable()
			if err != nil {
				t.Fatal("test executable unavailable")
			}
			cmd := exec.CommandContext(ctx, binary, "-test.run=^TestReviewDeliveryProcessHelper$", "-test.timeout=20s")
			cmd.Env = append(os.Environ(), "MEDIA_E2E_REVIEW_CHILD_MODE="+mode,
				"MEDIA_E2E_REVIEW_CHILD_DSN="+db.Dialector.(*pgdriver.Dialector).DSN, "MEDIA_E2E_REVIEW_CHILD_ID="+review.ID)
			if crashMode == "commit-and-exit" {
				cmd.Env = append(cmd.Env, "MEDIA_E2E_REVIEW_CHILD_ALREADY_COMMITTED=true")
			}
			// Capture, but never print, output that may include connection details.
			_, err = cmd.CombinedOutput()
			if expectedExit == 0 && err != nil {
				t.Fatal("review delivery subprocess failed")
			}
			if expectedExit != 0 {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != expectedExit {
					t.Fatal("review claim subprocess did not exit at crash point")
				}
			}
		}
		runChild(crashMode, 77)
		saved, err := store.GetReviewByKey(ctx, step.ID, "once")
		if err != nil || saved == nil || saved.Status != "pending" || saved.ClaimToken == "" {
			t.Fatal("crashed process did not leave a durable claimed review")
		}
		// Advance only the isolated fixture lease instead of sleeping two minutes.
		if err := db.Exec("UPDATE media_workflow.step_run_reviews SET claimed_at=now()-interval '3 minutes' WHERE id=?", review.ID).Error; err != nil {
			t.Fatal("isolated lease expiry setup failed")
		}
		runChild("deliver", 0)
		saved, err = store.GetReviewByKey(ctx, step.ID, "once")
		if err != nil || saved == nil || saved.Status != "applied" || saved.RequestDigest != review.RequestDigest {
			t.Fatal("new process did not recover the original durable decision")
		}
		var attempts int
		if err := db.Raw("SELECT delivery_attempts FROM media_workflow.step_run_reviews WHERE id=?", review.ID).Scan(&attempts).Error; err != nil || attempts != 2 {
			t.Fatal("new process did not reclaim exactly one abandoned lease")
		}
		execution, err := api.GetWorkflow(ctx, id)
		if err != nil || execution.Status != "COMPLETED" || len(execution.Tasks) != 1 || execution.Tasks[0].TaskID != taskID {
			t.Fatal("recovered decision did not complete the original isolated workflow")
		}
		return
	}
	engine := &committedReviewResponseLoss{WorkflowEngine: &conductor.ExecutionEngine{Client: api}}
	flow := &review_delivery.Flow{Reviews: store, Jobs: store, Engine: engine}
	if err := flow.DispatchOne(ctx); !errors.Is(err, review_delivery.ErrTemporarilyUnavailable) || engine.calls != 1 {
		t.Fatalf("response loss was not retained: %v", err)
	}
	if err := db.Exec("UPDATE media_workflow.step_run_reviews SET next_attempt_at=now()-interval '1 second' WHERE id=?", review.ID).Error; err != nil {
		t.Fatal(err)
	}
	restarted := repo.NewJobRepo(db)
	flow = &review_delivery.Flow{Reviews: restarted, Jobs: restarted, Engine: engine}
	if err := flow.DispatchOne(ctx); err != nil || engine.calls != 1 {
		t.Fatalf("restart repeated a committed decision: %v", err)
	}
	saved, err := restarted.GetReviewByKey(ctx, step.ID, "once")
	if err != nil || saved == nil || saved.Status != "applied" || saved.RequestDigest != review.RequestDigest {
		t.Fatalf("accepted decision was not recovered: %v", err)
	}
}

// Isolated PostgreSQL only: no user API, Conductor task, or real user record is
// changed. This gate does not prove delivery or HTTP approval acceptance.
func TestReviewOutboxPersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	txRunner := dal.NewTxRunner(db)
	store := repo.NewJobRepo(db)
	seed := func(name string) domain.ReviewDecision {
		t.Helper()
		candidates := []domain.Candidate{{Index: 0, Content: "candidate_" + name}}
		encoded, _ := json.Marshal(candidates)
		job := &model.Job{ID: "job_review_" + name, Scope: "e2e", WorkflowID: "workflow_review", WorkflowVersion: 1,
			Input: datatypes.JSON(`{}`), Status: "need_confirm", RetryHistory: datatypes.JSON(`[]`), SubmittedBy: "owner",
			SubmittedAt: time.Now(), ConductorWorkflowID: "execution_" + name}
		if err := db.Create(job).Error; err != nil {
			t.Fatal(err)
		}
		step := &model.StepRun{ID: "step_review_" + name, JobID: job.ID, StepKey: "image", StepOrder: 1, TemplateID: "test", TemplateVersion: 1,
			Status: "need_confirm", Candidates: datatypes.JSON(encoded)}
		if err := db.Create(step).Error; err != nil {
			t.Fatal(err)
		}
		index := 0
		output, _ := json.Marshal(map[string]any{"decision": "accept", "selected_index": 0, "selected_candidate": candidates[0]})
		review := domain.ReviewDecision{ID: "review_" + name, JobID: job.ID, StepRunID: step.ID,
			ConductorWorkflowID: job.ConductorWorkflowID, ConductorTaskID: "human_" + name, TaskReferenceName: "step_696d616765_approval__1", Generation: 1,
			Decision: "accept", DecidedBy: "owner", IdempotencyKey: "once", SelectedIndex: &index, CandidatesSnapshot: encoded, OutputJSON: output}
		review.RequestDigest, _ = review.Digest()
		return review
	}
	accept := func(review domain.ReviewDecision) (*domain.ReviewDecision, bool, error) {
		var saved *domain.ReviewDecision
		var created bool
		err := txRunner.RunInTx(ctx, func(txCtx context.Context) error {
			var err error
			saved, created, err = store.AcceptReview(txCtx, review)
			return err
		})
		return saved, created, err
	}
	count := func(stepID string) int64 {
		t.Helper()
		var value int64
		if err := db.Table("media_workflow.step_run_reviews").Where("step_run_id=?", stepID).Count(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	t.Run("transaction required and rollback", func(t *testing.T) {
		review := seed("rollback")
		if _, _, err := store.AcceptReview(ctx, review); err == nil {
			t.Fatal("approval writes accepted without aggregate transaction")
		}
		stop := errors.New("injected rollback")
		err := txRunner.RunInTx(ctx, func(txCtx context.Context) error {
			if _, _, err := store.AcceptReview(txCtx, review); err != nil {
				return err
			}
			return stop
		})
		if !errors.Is(err, stop) || count(review.StepRunID) != 0 {
			t.Fatalf("review did not roll back: %v", err)
		}
		var step model.StepRun
		var job model.Job
		if err := db.First(&step, "id=?", review.StepRunID).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.First(&job, "id=?", review.JobID).Error; err != nil {
			t.Fatal(err)
		}
		if step.Status != "need_confirm" || job.Status != "need_confirm" || step.SelectedIndex != nil {
			t.Fatal("aggregate state escaped rollback")
		}
	})
	t.Run("concurrent replay and conflicting payload", func(t *testing.T) {
		review := seed("replay")
		var wg sync.WaitGroup
		results := make(chan bool, 8)
		errorsSeen := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				command := review
				command.ID = fmt.Sprintf("replay_%d", i)
				_, created, err := accept(command)
				results <- created
				errorsSeen <- err
			}(i)
		}
		wg.Wait()
		close(results)
		close(errorsSeen)
		createdCount := 0
		for created := range results {
			if created {
				createdCount++
			}
		}
		for err := range errorsSeen {
			if err != nil {
				t.Fatal(err)
			}
		}
		if createdCount != 1 || count(review.StepRunID) != 1 {
			t.Fatal("concurrent retries created multiple decisions")
		}
		if err := db.Exec("UPDATE media_workflow.step_run_reviews SET selected_index=NULL WHERE step_run_id=?", review.StepRunID).Error; err == nil {
			t.Fatal("SQL constraint accepted an accept decision without a selection")
		}
		changed := review
		changed.Decision, changed.SelectedIndex = "regenerate", nil
		changed.OutputJSON = json.RawMessage(`{"decision":"regenerate"}`)
		changed.RequestDigest, _ = changed.Digest()
		if _, _, err := accept(changed); !errors.Is(err, domain.ErrReviewConflict) {
			t.Fatalf("same key changed decision: %v", err)
		}
		changed.IdempotencyKey = "other-key"
		changed.RequestDigest, _ = changed.Digest()
		if _, _, err := accept(changed); !errors.Is(err, domain.ErrReviewConflict) {
			t.Fatalf("opposite decision was accepted for the same generation: %v", err)
		}
	})
	t.Run("ownership and non-authoritative candidate projection", func(t *testing.T) {
		review := seed("authority")
		foreign := review
		foreign.DecidedBy = "other-owner"
		foreign.RequestDigest, _ = foreign.Digest()
		if _, _, err := accept(foreign); !errors.Is(err, domain.ErrReviewConflict) {
			t.Fatal("foreign owner stored a decision")
		}
		if err := db.Model(&model.StepRun{}).Where("id=?", review.StepRunID).Update("candidates", datatypes.JSON(`[{"content":"new-round"}]`)).Error; err != nil {
			t.Fatal(err)
		}
		if _, _, err := accept(review); err != nil || count(review.StepRunID) != 1 {
			t.Fatal("cached candidate drift vetoed native frozen decision", err)
		}
	})
	t.Run("user command and stale waiting projection", func(t *testing.T) {
		review := seed("projection")
		if err := db.Model(&model.Job{}).Where("id=?", review.JobID).Update("workspace_id", "workspace").Error; err != nil {
			t.Fatal(err)
		}
		var candidates []domain.Candidate
		if err := json.Unmarshal(review.CandidatesSnapshot, &candidates); err != nil {
			t.Fatal(err)
		}
		engine := reviewProjectionEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{{TaskID: review.ConductorTaskID, TaskType: "HUMAN", ReferenceTaskName: review.TaskReferenceName, Status: "IN_PROGRESS", Input: map[string]any{"candidates": candidates}}}}}
		svc := jobservice.NewService(store, repo.NewStepRunRepo(db), txRunner, nil, nil, nil, nil, nil, engine)
		svc.ConfigureReviews(store)
		svc.ConfigureResume(store, reviewWorkspaceAccess{}, nil, "")
		cmd := jobservice.ReviewCommand{StepRunID: review.StepRunID, UserID: "owner", IdempotencyKey: "user-request", Decision: "accept", SelectedIndex: 0}
		receipt, err := svc.DecideReview(ctx, cmd)
		if err != nil || receipt.LastReviewID == "" || receipt.LastReviewStatus != "pending" {
			t.Fatalf("user command failed: %v", err)
		}
		// Receipt state is separate from native waiting state; accepting locally
		// must not claim a Step or advance Job execution before engine delivery.
		j, steps, err := svc.Get(ctx, review.JobID)
		if err != nil || j.Status != domain.JobNeedConfirm || len(steps) != 1 || steps[0].Status != domain.SRNeedConfirm || steps[0].LastReviewID != receipt.LastReviewID {
			t.Fatalf("stale read re-exposed submitted review: %v", err)
		}
		if err := db.Model(&model.Job{}).Where("id=?", review.JobID).Update("status", "succeeded").Error; err != nil {
			t.Fatal(err)
		}
		engine.execution.Status = "COMPLETED"
		again, err := svc.DecideReview(ctx, cmd)
		if err != nil || again.LastReviewID != receipt.LastReviewID || count(review.StepRunID) != 1 {
			t.Fatalf("terminal request did not replay: %v", err)
		}
		cmd.Decision = "regenerate"
		if _, err := svc.DecideReview(ctx, cmd); !errors.Is(err, domain.ErrReviewConflict) {
			t.Fatal("changed terminal request did not conflict", err)
		}
	})
	t.Run("new generation and terminal replay", func(t *testing.T) {
		review := seed("generation")
		if _, _, err := accept(review); err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&model.StepRun{}).Where("id=?", review.StepRunID).Update("status", "need_confirm").Error; err != nil {
			t.Fatal(err)
		}
		next := review
		next.ID, next.ConductorTaskID, next.TaskReferenceName, next.Generation = "review_next", "human_next", "step_696d616765_approval__2", 2
		next.RequestDigest, _ = next.Digest()
		if _, _, err := accept(next); !errors.Is(err, domain.ErrReviewConflict) {
			t.Fatal("an old command key was rebound to the new generation")
		}
		next.IdempotencyKey = "second-generation"
		next.RequestDigest, _ = next.Digest()
		if _, created, err := accept(next); err != nil || !created || count(review.StepRunID) != 2 {
			t.Fatalf("new generation not stored independently: %v", err)
		}
		if err := db.Model(&model.Job{}).Where("id=?", review.JobID).Update("status", "succeeded").Error; err != nil {
			t.Fatal(err)
		}
		if saved, created, err := accept(review); err != nil || created || saved.ConductorTaskID != review.ConductorTaskID {
			t.Fatalf("terminal replay did not return the original decision: %v", err)
		}
	})
	// Earlier subtests intentionally left commands pending; mark only this
	// isolated database's rows applied so the lease scenario has one target.
	if err := db.Exec("UPDATE media_workflow.step_run_reviews SET status='applied'").Error; err != nil {
		t.Fatal(err)
	}
	t.Run("restart lease recovery fences old claim", func(t *testing.T) {
		review := seed("claim")
		if _, _, err := accept(review); err != nil {
			t.Fatal(err)
		}
		first, err := store.ClaimPendingReview(ctx)
		if err != nil || first == nil || first.ClaimToken == "" || first.ID != review.ID {
			t.Fatalf("command not claimed: %v", err)
		}
		if duplicate, err := store.ClaimPendingReview(ctx); err != nil || duplicate != nil {
			t.Fatal("live command lease was claimed twice")
		}
		if err := store.ReleaseReviewClaim(ctx, *first, "upstream https://secret.invalid/token"); err == nil {
			t.Fatal("raw upstream error was stored instead of a stable code")
		}
		if err := db.Exec("UPDATE media_workflow.step_run_reviews SET claimed_at=now()-interval '3 minutes' WHERE id=?", review.ID).Error; err != nil {
			t.Fatal(err)
		}
		restarted := repo.NewJobRepo(db)
		second, err := restarted.ClaimPendingReview(ctx)
		if err != nil || second == nil || second.ID != first.ID || second.ClaimToken == first.ClaimToken || second.RequestDigest != first.RequestDigest {
			t.Fatalf("restart did not reclaim the frozen command: %v", err)
		}
		if err := store.FinishReview(ctx, *first, "applied", ""); !errors.Is(err, domain.ErrReviewConflict) {
			t.Fatal("stale process completed a newer claim")
		}
		if err := store.ReleaseReviewClaim(ctx, *first, "temporary_unavailable"); !errors.Is(err, domain.ErrReviewConflict) {
			t.Fatal("stale process released a newer claim")
		}
		if err := restarted.ReleaseReviewClaim(ctx, *second, "temporary_unavailable"); err != nil {
			t.Fatal(err)
		}
		if delayed, err := restarted.ClaimPendingReview(ctx); err != nil || delayed != nil {
			t.Fatal("delivery backoff was bypassed")
		}
		if err := db.Exec("UPDATE media_workflow.step_run_reviews SET next_attempt_at=now()-interval '1 second' WHERE id=?", review.ID).Error; err != nil {
			t.Fatal(err)
		}
		third, err := restarted.ClaimPendingReview(ctx)
		if err != nil || third == nil {
			t.Fatalf("released command not retried: %v", err)
		}
		if err := restarted.FinishReview(ctx, *third, "applied", ""); err != nil {
			t.Fatal(err)
		}
		if done, err := restarted.ClaimPendingReview(ctx); err != nil || done != nil {
			t.Fatal("applied command was claimed again")
		}
	})
	t.Run("migration rollback preserves audit", func(t *testing.T) {
		script, err := os.ReadFile("../migrations/000016_step_run_reviews.down.sql")
		if err != nil {
			t.Fatal(err)
		}
		err = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(script)).Error })
		if err == nil {
			t.Fatal("rollback discarded existing review audit")
		}
		var total int64
		if err := db.Table("media_workflow.step_run_reviews").Count(&total).Error; err != nil || total < 4 {
			t.Fatalf("review audit was lost during rollback: %v", err)
		}
	})
}
