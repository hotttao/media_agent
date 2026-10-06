package review_delivery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

type reviewStore struct {
	domain.ReviewRepo
	review         domain.ReviewDecision
	status, reason string
	claimed        bool
}

func (s *reviewStore) ClaimPendingReview(context.Context) (*domain.ReviewDecision, error) {
	if s.claimed || s.status != "pending" {
		return nil, nil
	}
	s.claimed = true
	copy := s.review
	copy.ClaimToken = "claim"
	return &copy, nil
}
func (s *reviewStore) FinishReview(_ context.Context, _ domain.ReviewDecision, status, reason string) error {
	s.status, s.reason = status, reason
	return nil
}
func (s *reviewStore) ReleaseReviewClaim(_ context.Context, _ domain.ReviewDecision, reason string) error {
	s.claimed, s.reason = false, reason
	return nil
}

type jobs struct {
	domain.JobRepo
	job domain.Job
}

func (j jobs) Get(context.Context, string) (*domain.Job, error) { return &j.job, nil }

type engine struct {
	domain.WorkflowEngine
	execution     domain.EngineExecution
	completeCalls int
	loseResponse  bool
}

func (e *engine) Get(context.Context, string) (*domain.EngineExecution, error) {
	return &e.execution, nil
}
func (e *engine) CompleteApprovalTask(_ context.Context, _ string, taskID string, output map[string]any) error {
	e.completeCalls++
	if taskID != e.execution.Tasks[0].TaskID {
		return errors.New("wrong immutable task identity")
	}
	e.execution.Tasks[0].Status, e.execution.Tasks[0].Output = "COMPLETED", output
	e.execution.Status = "COMPLETED"
	if e.loseResponse {
		return errors.New("secret upstream response must not escape")
	}
	return nil
}

func fixture() (*reviewStore, jobs, *engine) {
	index := 0
	review := domain.ReviewDecision{ID: "review", JobID: "job", StepRunID: "step", ConductorWorkflowID: "execution", ConductorTaskID: "human", TaskReferenceName: "approval__1",
		Generation: 1, Decision: "accept", DecidedBy: "owner", IdempotencyKey: "once", SelectedIndex: &index,
		CandidatesSnapshot: json.RawMessage(`[{"content":"image"}]`),
		OutputJSON:         json.RawMessage(`{"decision":"accept","selected_index":0,"selected_candidate":{"content":"image"}}`)}
	review.RequestDigest, _ = review.Digest()
	return &reviewStore{review: review, status: "pending"}, jobs{job: domain.Job{ID: "job", SubmittedBy: "owner", Status: domain.JobRunning, ConductorWorkflowID: "execution"}},
		&engine{execution: domain.EngineExecution{Status: "RUNNING", Tasks: []domain.EngineTask{{TaskID: "human", TaskType: "HUMAN", ReferenceTaskName: "approval__1", Status: "IN_PROGRESS"}}}}
}

func TestDeliveryRecoversCommittedDecisionWithLostResponse(t *testing.T) {
	store, jobs, engine := fixture()
	engine.loseResponse = true
	flow := &Flow{Reviews: store, Jobs: jobs, Engine: engine}
	if err := flow.DispatchOne(context.Background()); !errors.Is(err, ErrTemporarilyUnavailable) || strings.Contains(err.Error(), "secret") || store.reason != "task_completion_unavailable" {
		t.Fatal("uncertain response was not safely retained")
	}
	// A reconstructed dispatcher queries the frozen Task ID before sending.
	flow = &Flow{Reviews: store, Jobs: jobs, Engine: engine}
	if err := flow.DispatchOne(context.Background()); err != nil || store.status != "applied" || engine.completeCalls != 1 {
		t.Fatalf("committed decision was resubmitted or lost: %v", err)
	}
}

func TestDeliveryDoesNotSubmitReviewFromEarlierRetryCycle(t *testing.T) {
	store, jobs, engine := fixture()
	epoch := 0
	store.review.AcceptedRetryCount = &epoch
	store.review.RequestDigest, _ = store.review.Digest()
	jobs.job.RetryCount = 1
	flow := &Flow{Reviews: store, Jobs: jobs, Engine: engine}
	if err := flow.DispatchOne(context.Background()); err != nil || store.status != "failed" || store.reason != "execution_retried" || engine.completeCalls != 0 {
		t.Fatal("old cycle review submitted to restarted execution")
	}
}

func TestDeliveryDoesNotInventEpochForLegacyPendingReview(t *testing.T) {
	store, jobs, engine := fixture()
	jobs.job.RetryCount = 1
	flow := &Flow{Reviews: store, Jobs: jobs, Engine: engine}
	if err := flow.DispatchOne(context.Background()); err != nil || store.status != "failed" || store.reason != "unknown_retry_epoch" || engine.completeCalls != 0 {
		t.Fatal("legacy unknown cycle decision was submitted after retry")
	}
}

func TestDeliveryFailsClosedWithoutOverwritingOtherDecision(t *testing.T) {
	for _, name := range []string{"owner changed", "execution changed", "task changed", "cancelled", "different completed output"} {
		t.Run(name, func(t *testing.T) {
			store, jobs, engine := fixture()
			switch name {
			case "owner changed":
				jobs.job.SubmittedBy = "other"
			case "execution changed":
				jobs.job.ConductorWorkflowID = "replacement"
			case "task changed":
				engine.execution.Tasks[0].TaskID = "new-human"
			case "cancelled":
				jobs.job.Status = domain.JobCancelled
			case "different completed output":
				engine.execution.Tasks[0].Status = "COMPLETED"
				engine.execution.Tasks[0].Output = map[string]any{"decision": "regenerate"}
			}
			flow := &Flow{Reviews: store, Jobs: jobs, Engine: engine}
			if err := flow.DispatchOne(context.Background()); err != nil || store.status != "failed" || engine.completeCalls != 0 {
				t.Fatalf("invalid target received a decision: %v", err)
			}
		})
	}
}
