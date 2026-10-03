package job

import (
	domain "media_agent/media_workflow/biz/domain/job"
	"testing"
	"time"
)

func TestExecutionResponsesPreserveHistoricalAndRuntimeTimes(t *testing.T) {
	created := time.Date(2026, 5, 1, 0, 0, 0, 123000000, time.UTC)
	finished := created.Add(time.Minute)
	job := jobToPB(&domain.Job{SubmittedAt: created, StartedAt: &created, FinishedAt: &finished, FailureReason: "example failure"})
	if job.GetStartedAt() != created.UnixNano() || job.GetFinishedAt() != finished.UnixNano() || job.GetFailureReason() != "example failure" {
		t.Fatal("Job execution facts were omitted")
	}
	step := stepRunToPB(&domain.StepRun{SourceCreatedAt: &created, ClaimedAt: &created, StartedAt: &created, FinishedAt: &finished})
	if step.GetSourceCreatedAt() != created.UnixNano() || step.GetClaimedAt() != created.UnixNano() || step.GetStartedAt() != created.UnixNano() || step.GetFinishedAt() != finished.UnixNano() {
		t.Fatal("StepRun execution facts were omitted")
	}
	empty := jobToPB(&domain.Job{})
	if empty.GetStartedAt() != 0 || empty.GetFinishedAt() != 0 {
		t.Fatal("absent execution time was fabricated")
	}
}
