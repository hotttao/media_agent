package finalize_task

import (
	"errors"
	"testing"
)

func TestFinalizeSingleCandidate(t *testing.T) {
	out, err := selectCandidate(Input{JobID: "job-1", StepKey: "video", OutputAssetKind: "final_video", Candidates: []Candidate{{URL: "s3://bucket/video.mp4"}}})
	if err != nil || out.URL != "s3://bucket/video.mp4" {
		t.Fatalf("unexpected result: %#v err=%v", out, err)
	}
}

func TestFinalizeRequiresApproval(t *testing.T) {
	_, err := selectCandidate(Input{Candidates: []Candidate{{URL: "a"}, {URL: "b"}}})
	if !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("expected approval required, got %v", err)
	}
}

func TestFinalizeUsesApprovedCandidate(t *testing.T) {
	selected := Candidate{URL: "approved.png"}
	out, err := selectCandidate(Input{
		JobID: "job-1", StepKey: "image", OutputAssetKind: "image", ApprovalPolicy: "always",
		Candidates: []Candidate{{URL: "first.png"}, selected}, SelectedCandidate: &selected,
	})
	if err != nil || out.URL != "approved.png" {
		t.Fatalf("unexpected approved result: %#v err=%v", out, err)
	}
}

func TestFinalizeRejectsCandidateFromPreviousGeneration(t *testing.T) {
	old := Candidate{URL: "old.png"}
	_, err := selectCandidate(Input{Candidates: []Candidate{{URL: "current.png"}}, SelectedCandidate: &old})
	if err == nil {
		t.Fatal("candidate from an earlier round was finalized")
	}
}
