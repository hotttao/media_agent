package restore_task

import (
	"context"
	"strings"
	"testing"
	"time"

	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type fixtureJob struct{ job *jobdomain.Job }

func (f fixtureJob) Get(context.Context, string) (*jobdomain.Job, error) { return f.job, nil }

type fixtureStep struct{ step *jobdomain.StepRun }

func (f fixtureStep) GetByJobAndStepKey(context.Context, string, string) (*jobdomain.StepRun, error) {
	return f.step, nil
}

type fixtureAsset struct {
	asset *client.ImportedGeneratedAsset
}

func (f fixtureAsset) GetImportedGeneratedAsset(context.Context, string) (*client.ImportedGeneratedAsset, error) {
	return f.asset, nil
}

func TestRestoreRequiresFrozenHistoricalFacts(t *testing.T) {
	created := time.Date(2026, 5, 27, 4, 51, 30, 0, time.UTC)
	job := &jobdomain.Job{ID: "job", Origin: "legacy_import", OrganizationID: "org", Scope: "ai_clothing",
		SubmittedBy: "owner", Status: jobdomain.JobRunning, ResumeGeneration: 1}
	step := &jobdomain.StepRun{JobID: job.ID, StepKey: "first_frame", Origin: "legacy_import", Status: jobdomain.SRSucceeded,
		OutputAssetID: "gen", AssetKind: "first_frame", SourceSystem: "media_ai", SourceRecordID: "frame",
		SourceSHA256: strings.Repeat("a", 64), SourceCreatedAt: &created}
	asset := &client.ImportedGeneratedAsset{ID: "gen", OrganizationID: "org", Scope: "ai_clothing", OwnerUserID: "owner",
		AssetKind: "first_frame", SourceSystem: "media_ai", SourceRecordID: "frame", SourceSHA256: step.SourceSHA256,
		SourceCreatedAt: created.UnixNano(), AccessURL: "https://storage.example/frame.png", AccessExpiresAt: time.Now().Add(time.Minute).Unix()}
	handler := Handler{Jobs: fixtureJob{job}, Steps: fixtureStep{step}, Assets: fixtureAsset{asset}}
	output, err := handler.Execute(context.Background(), Input{JobID: job.ID, StepKey: step.StepKey})
	if err != nil || output.OutputResourceID != "gen" || output.OutputResource.AssetRef != "gen" {
		t.Fatalf("restore output=%+v err=%v", output, err)
	}
	asset.OrganizationID = "other"
	if _, err := handler.Execute(context.Background(), Input{JobID: job.ID, StepKey: step.StepKey}); err == nil {
		t.Fatal("cross-organization asset restored")
	}
	asset.OrganizationID = "org"
	job.Status = jobdomain.JobAwaitingResume
	if _, err := handler.Execute(context.Background(), Input{JobID: job.ID, StepKey: step.StepKey}); err == nil {
		t.Fatal("unresumed Job restored")
	}
}
