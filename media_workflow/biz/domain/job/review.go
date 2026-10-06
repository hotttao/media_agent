package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
)

var ErrReviewConflict = errors.New("approval decision conflicts with the current generation")

// ReviewDecision is both the immutable user decision and its durable delivery
// command. ClaimToken belongs to a delivery lease, not the decision identity.
type ReviewDecision struct {
	ID, JobID, StepRunID                                    string
	ConductorWorkflowID, ConductorTaskID, TaskReferenceName string
	Generation                                              int
	AcceptedRetryCount                                      *int
	Decision, DecidedBy, IdempotencyKey, Feedback           string
	SelectedIndex                                           *int
	CandidatesSnapshot, OutputJSON                          json.RawMessage
	RequestDigest, Status, ClaimToken                       string
}

func (r ReviewDecision) Digest() (string, error) {
	var candidates, output any
	if err := json.Unmarshal(r.CandidatesSnapshot, &candidates); err != nil {
		return "", err
	}
	if err := json.Unmarshal(r.OutputJSON, &output); err != nil {
		return "", err
	}
	values := []any{r.JobID, r.StepRunID, r.ConductorWorkflowID, r.ConductorTaskID, r.TaskReferenceName,
		r.Generation, r.Decision, r.DecidedBy, r.IdempotencyKey, r.Feedback, r.SelectedIndex, candidates, output}
	if r.AcceptedRetryCount != nil {
		values = append(values, map[string]int{"accepted_retry_count": *r.AcceptedRetryCount})
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (r ReviewDecision) Validate() error {
	if r.AcceptedRetryCount != nil && *r.AcceptedRetryCount < 0 {
		return errors.New("invalid approval retry epoch")
	}
	if r.ID == "" || r.JobID == "" || r.StepRunID == "" || r.ConductorWorkflowID == "" || r.ConductorTaskID == "" ||
		r.TaskReferenceName == "" || r.DecidedBy == "" || r.Generation < 0 || len(r.IdempotencyKey) == 0 || len(r.IdempotencyKey) > 160 ||
		len(r.Feedback) > 8192 || len(r.CandidatesSnapshot) > 1048576 || len(r.OutputJSON) > 1048576 {
		return errors.New("invalid approval decision identity or size")
	}
	var candidates []Candidate
	var output map[string]json.RawMessage
	if json.Unmarshal(r.CandidatesSnapshot, &candidates) != nil || len(candidates) == 0 || json.Unmarshal(r.OutputJSON, &output) != nil {
		return errors.New("invalid approval decision snapshot")
	}
	var decision string
	if json.Unmarshal(output["decision"], &decision) != nil || decision != r.Decision {
		return errors.New("approval output does not match decision")
	}
	switch r.Decision {
	case "accept":
		if r.SelectedIndex == nil || *r.SelectedIndex < 0 || *r.SelectedIndex >= len(candidates) || r.Feedback != "" {
			return errors.New("invalid approval candidate selection")
		}
		var selected Candidate
		var index int
		if json.Unmarshal(output["selected_candidate"], &selected) != nil || selected != candidates[*r.SelectedIndex] ||
			json.Unmarshal(output["selected_index"], &index) != nil || index != *r.SelectedIndex {
			return errors.New("selected candidate does not match snapshot")
		}
	case "regenerate":
		if r.Generation == 0 || r.SelectedIndex != nil || output["selected_candidate"] != nil || output["selected_index"] != nil {
			return errors.New("regeneration must not accept a candidate")
		}
		var feedback string
		if raw := output["feedback"]; raw != nil && json.Unmarshal(raw, &feedback) != nil {
			return errors.New("invalid regeneration feedback")
		}
		if feedback != r.Feedback {
			return errors.New("regeneration feedback does not match decision")
		}
	default:
		return errors.New("unknown approval decision")
	}
	digest, err := r.Digest()
	if err != nil || digest != r.RequestDigest {
		return errors.New("approval decision digest mismatch")
	}
	return nil
}

func (r ReviewDecision) MatchesCandidates(candidates []Candidate) bool {
	var frozen []Candidate
	return json.Unmarshal(r.CandidatesSnapshot, &frozen) == nil && reflect.DeepEqual(frozen, candidates)
}

type ReviewRepo interface {
	// AcceptReview must run inside the caller's aggregate transaction. It locks
	// Job then StepRun and atomically stores the command and local state.
	AcceptReview(ctx context.Context, review ReviewDecision) (*ReviewDecision, bool, error)
	GetReviewByKey(ctx context.Context, stepRunID, key string) (*ReviewDecision, error)
	GetReviewByTask(ctx context.Context, stepRunID, taskID string) (*ReviewDecision, error)
	LatestReviewsByJob(ctx context.Context, jobID string) ([]ReviewDecision, error)
	// LockReviewProjection serializes local projection with AcceptReview.
	// The caller must hold a transaction; no engine calls may run under it.
	LockReviewProjection(ctx context.Context, jobID, stepRunID string) error
	ClaimPendingReview(ctx context.Context) (*ReviewDecision, error)
	FinishReview(ctx context.Context, review ReviewDecision, status, reasonCode string) error
	ReleaseReviewClaim(ctx context.Context, review ReviewDecision, reasonCode string) error
}
