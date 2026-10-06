package job

import (
	"encoding/json"
	"testing"
)

func validReview() ReviewDecision {
	index := 0
	r := ReviewDecision{ID: "review", JobID: "job", StepRunID: "step", ConductorWorkflowID: "workflow", ConductorTaskID: "human",
		TaskReferenceName: "approval__1", Generation: 1, Decision: "accept", DecidedBy: "owner", IdempotencyKey: "once", SelectedIndex: &index,
		CandidatesSnapshot: json.RawMessage(`[{"index":0,"content":"image"}]`),
		OutputJSON:         json.RawMessage(`{"decision":"accept","selected_index":0,"selected_candidate":{"index":0,"content":"image"}}`)}
	r.RequestDigest, _ = r.Digest()
	return r
}

func TestReviewDigestCanonicalAndGenerationBound(t *testing.T) {
	r := validReview()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	same := r
	same.OutputJSON = json.RawMessage(`{ "selected_candidate": {"content":"image", "index":0}, "selected_index":0, "decision":"accept" }`)
	digest, err := same.Digest()
	if err != nil || digest != r.RequestDigest {
		t.Fatal("JSON formatting changed immutable decision identity")
	}
	same.ID, same.ClaimToken, same.Status = "another-insert-id", "lease-two", "applied"
	digest, _ = same.Digest()
	if digest != r.RequestDigest {
		t.Fatal("delivery metadata changed command identity")
	}
	same.ConductorTaskID, same.Generation = "next-human", 2
	digest, _ = same.Digest()
	if digest == r.RequestDigest {
		t.Fatal("new generation reused old decision identity")
	}
}

func TestReviewRejectsForgedSelectionAndRegenerationAcceptance(t *testing.T) {
	for _, mutate := range []func(*ReviewDecision){
		func(r *ReviewDecision) {
			r.OutputJSON = json.RawMessage(`{"decision":"accept","selected_index":0,"selected_candidate":{"content":"other"}}`)
		},
		func(r *ReviewDecision) { r.Decision = "regenerate" },
		func(r *ReviewDecision) { r.SelectedIndex = nil },
		func(r *ReviewDecision) { r.RequestDigest = "wrong" },
		func(r *ReviewDecision) { r.Generation = -1 },
	} {
		r := validReview()
		mutate(&r)
		if r.Validate() == nil {
			t.Fatal("invalid decision passed validation")
		}
	}
	r := validReview()
	r.Decision, r.SelectedIndex, r.Feedback = "regenerate", nil, "use a different pose"
	r.OutputJSON = json.RawMessage(`{"decision":"regenerate","feedback":"use a different pose"}`)
	r.RequestDigest, _ = r.Digest()
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Generation = 0
	r.RequestDigest, _ = r.Digest()
	if r.Validate() == nil {
		t.Fatal("legacy approval accepted a regeneration decision")
	}
}

func TestReviewRetryEpochDigestAndLegacyCompatibility(t *testing.T) {
	r := validReview()
	legacy := r.RequestDigest
	if digest, err := r.Digest(); err != nil || digest != legacy {
		t.Fatal("legacy digest changed")
	}
	epoch := 0
	r.AcceptedRetryCount = &epoch
	r.RequestDigest, _ = r.Digest()
	if r.RequestDigest == legacy || r.Validate() != nil {
		t.Fatal("new epoch did not bind decision")
	}
	epoch = 1
	if r.Validate() == nil {
		t.Fatal("epoch changed without updating digest")
	}
	r.RequestDigest, _ = r.Digest()
	if r.Validate() != nil {
		t.Fatal("new cycle digest rejected")
	}
	epoch = -1
	r.RequestDigest, _ = r.Digest()
	if r.Validate() == nil {
		t.Fatal("negative epoch accepted")
	}
}
