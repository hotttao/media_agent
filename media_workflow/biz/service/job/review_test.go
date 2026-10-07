package job

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"media_agent/hertz_infra/authn/internaljwt"
	domain "media_agent/media_workflow/biz/domain/job"
)

func TestReviewUsesNativeCandidatesDespiteStaleProjection(t *testing.T) {
	for _, status := range []domain.JobStatus{domain.JobFailed, domain.JobSucceeded, domain.JobRunning} {
		t.Run(string(status), func(t *testing.T) {
			svc, reviews, engine, cmd := reviewCommandFixture()
			svc.jobRepo.(*mockJobRepo).jobs["job1"].Status = status
			svc.stepRunRepo.(*mockStepRunRepo).items[cmd.StepRunID].Status = domain.SRFailed
			current := []domain.Candidate{{Content: "native current result"}}
			engine.execution.Tasks[0].Input["candidates"] = current
			if _, err := svc.DecideReview(context.Background(), cmd); err != nil {
				t.Fatal("projection incorrectly vetoed native approval", err)
			}
			var output struct {
				SelectedCandidate domain.Candidate `json:"selected_candidate"`
			}
			if json.Unmarshal(reviews.review.OutputJSON, &output) != nil || output.SelectedCandidate != current[0] {
				t.Fatal("decision selected cached rather than native candidate")
			}
		})
	}
}

type commandReviewStore struct {
	domain.ReviewRepo
	review *domain.ReviewDecision
	steps  *mockStepRunRepo
	writes int
}

type commandReviewTx struct{}

func (*commandReviewStore) LockReviewProjection(context.Context, string, string) error { return nil }

func TestCompletedApprovalProjectsSelectionOnlyAfterNativeCompletion(t *testing.T) {
	svc, _, engine, cmd := reviewCommandFixture()
	if _, err := svc.DecideReview(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	step := svc.stepRunRepo.(*mockStepRunRepo).items[cmd.StepRunID]
	if step.SelectedIndex != nil || step.Status != domain.SRNeedConfirm {
		t.Fatal("local acceptance advanced execution")
	}
	task := engine.execution.Tasks[0]
	task.Status = "COMPLETED"
	task.Output = map[string]any{"decision": "accept", "selected_index": 0}
	if err := svc.projectCompletedReview(context.Background(), "job1", cmd.StepRunID, task); err != nil {
		t.Fatal(err)
	}
	if step.SelectedIndex == nil || *step.SelectedIndex != 0 || step.Status != domain.SRClaimed {
		t.Fatal("native completed selection not projected")
	}
}

func (commandReviewTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (r *commandReviewStore) GetReviewByKey(_ context.Context, step, key string) (*domain.ReviewDecision, error) {
	if r.review != nil && r.review.StepRunID == step && r.review.IdempotencyKey == key {
		return r.review, nil
	}
	return nil, nil
}
func (r *commandReviewStore) AcceptReview(_ context.Context, review domain.ReviewDecision) (*domain.ReviewDecision, bool, error) {
	if err := review.Validate(); err != nil {
		return nil, false, err
	}
	if r.review != nil {
		return nil, false, domain.ErrReviewConflict
	}
	r.review, r.writes = &review, r.writes+1
	return r.review, true, nil
}
func (r *commandReviewStore) LatestReviewsByJob(context.Context, string) ([]domain.ReviewDecision, error) {
	if r.review == nil {
		return nil, nil
	}
	return []domain.ReviewDecision{*r.review}, nil
}

func reviewCommandFixture() (*Service, *commandReviewStore, *mockWorkflowEngine, ReviewCommand) {
	svc, jobs, steps := newTestService(false)
	sr := makeClaimedStepRun(steps, false)
	sr.Status, sr.Candidates = domain.SRNeedConfirm, []domain.Candidate{{Index: 7, Content: "candidate"}}
	j := &domain.Job{ID: "job1", SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "workspace", Status: domain.JobNeedConfirm, ConductorWorkflowID: "execution"}
	jobs.jobs[j.ID], jobs.stepRuns[j.ID] = j, []*domain.StepRun{sr}
	engine := &mockWorkflowEngine{execution: &domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{{TaskID: "task", TaskType: "HUMAN", ReferenceTaskName: "step_7331_approval__2", Status: "IN_PROGRESS", Input: map[string]any{"candidates": sr.Candidates}}}}}
	svc.engine, svc.resumeAccess = engine, fakeResumeAccess{allowed: true}
	svc.tx = commandReviewTx{}
	reviews := &commandReviewStore{steps: steps}
	svc.ConfigureReviews(reviews)
	return svc, reviews, engine, ReviewCommand{StepRunID: sr.ID, UserID: "owner", OrganizationID: "org", IdempotencyKey: "request-1", Decision: "accept", SelectedIndex: 0}
}

func TestReviewCommandPersistsBeforeDeliveryAndReplaysAfterCompletion(t *testing.T) {
	svc, reviews, engine, cmd := reviewCommandFixture()
	receipt, err := svc.DecideReview(context.Background(), cmd)
	if err != nil || receipt.LastReviewID == "" || receipt.LastReviewStatus != "pending" || receipt.Status != domain.SRNeedConfirm {
		t.Fatalf("decision not accepted: %v", err)
	}
	if engine.completedRef != "" || reviews.writes != 1 || *reviews.review.SelectedIndex != 0 || reviews.review.Generation != 2 {
		t.Fatal("command submitted remotely or did not freeze exact round/array position")
	}
	if reviews.review.AcceptedRetryCount == nil || *reviews.review.AcceptedRetryCount != 0 {
		t.Fatal("new approval did not freeze retry epoch")
	}
	svc.jobRepo.(*mockJobRepo).jobs["job1"].Status = domain.JobSucceeded
	engine.execution = nil // Replay must not need Conductor.
	again, err := svc.DecideReview(context.Background(), cmd)
	if err != nil || again.LastReviewID != receipt.LastReviewID || reviews.writes != 1 {
		t.Fatal("completed workflow did not replay original receipt", err)
	}
	cmd.Decision = "regenerate"
	if _, err := svc.DecideReview(context.Background(), cmd); !errors.Is(err, domain.ErrReviewConflict) {
		t.Fatal("changed request reused idempotency key", err)
	}
}

func TestReviewCommandRejectsUnauthorizedOrUnsupportedInputs(t *testing.T) {
	for _, kind := range []string{"owner", "organization", "permission", "feedback", "stale", "index"} {
		t.Run(kind, func(t *testing.T) {
			svc, reviews, engine, cmd := reviewCommandFixture()
			switch kind {
			case "owner":
				cmd.UserID = "other"
			case "organization":
				cmd.OrganizationID = "other"
			case "permission":
				svc.resumeAccess = fakeResumeAccess{}
			case "feedback":
				cmd.Decision, cmd.Feedback = "regenerate", "must not silently drop"
			case "stale":
				engine.execution.Tasks[0].Retried = true
			case "index":
				cmd.SelectedIndex = 7 // Candidate.Index is not its array position.
			}
			if _, err := svc.DecideReview(context.Background(), cmd); err == nil || reviews.writes != 0 || engine.completedRef != "" {
				t.Fatal("invalid command changed execution", err)
			}
		})
	}
}

func TestRegenerationCommandFreezesOnlyCurrentDecision(t *testing.T) {
	svc, reviews, engine, cmd := reviewCommandFixture()
	cmd.Decision, cmd.IdempotencyKey = "regenerate", "regenerate-1"
	if _, err := svc.DecideReview(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if reviews.review.SelectedIndex != nil || engine.rerunTaskID != "" || engine.completedRef != "" || engine.terminated {
		t.Fatal("regeneration accepted a candidate or mutated remote workflow")
	}
}

func TestReviewCommandRejectsTrustedPrincipalWorkspaceMismatch(t *testing.T) {
	svc, reviews, _, cmd := reviewCommandFixture()
	ctx := internaljwt.ContextWithAgentPrincipal(context.Background(), internaljwt.AgentPrincipal{UserID: "owner", OrganizationID: "org", WorkspaceID: "other-workspace"})
	if _, err := svc.DecideReview(ctx, cmd); !errors.Is(err, ErrSubmitForbidden) || reviews.writes != 0 {
		t.Fatal("trusted principal crossed Workspace boundary", err)
	}
}

func TestRegenerationFeedbackIsFrozenAndPartOfReplayIdentity(t *testing.T) {
	svc, reviews, engine, cmd := reviewCommandFixture()
	cmd.Decision, cmd.Feedback = "regenerate", "更亮 {{.not_a_template}}"
	engine.execution.Tasks[0].Input["feedback_mode"] = "last_regeneration_v1"
	receipt, err := svc.DecideReview(context.Background(), cmd)
	if err != nil || receipt.LastReviewID == "" || reviews.review.Feedback != cmd.Feedback {
		t.Fatal("feedback not accepted and frozen", err)
	}
	if err := reviews.review.Validate(); err != nil {
		t.Fatal(err)
	}
	engine.execution = nil
	if _, err := svc.DecideReview(context.Background(), cmd); err != nil {
		t.Fatal("feedback replay required engine", err)
	}
	cmd.Feedback = "changed"
	if _, err := svc.DecideReview(context.Background(), cmd); !errors.Is(err, domain.ErrReviewConflict) {
		t.Fatal("changed feedback replayed original command", err)
	}
}
