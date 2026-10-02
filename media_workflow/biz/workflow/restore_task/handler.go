package restore_task

import (
	"context"
	"errors"
	"fmt"
	"time"

	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/finalize_task"
)

const TaskDefinitionName = "media_restore_historical_step_v1"

type JobLookup interface {
	Get(context.Context, string) (*jobdomain.Job, error)
}
type StepLookup interface {
	GetByJobAndStepKey(context.Context, string, string) (*jobdomain.StepRun, error)
}
type AssetLookup interface {
	GetImportedGeneratedAsset(context.Context, string) (*client.ImportedGeneratedAsset, error)
}

type Input struct {
	JobID   string `json:"job_id"`
	StepKey string `json:"step_key"`
}

type Handler struct {
	Jobs   JobLookup
	Steps  StepLookup
	Assets AssetLookup
}

// Execute restores only a previously verified, successful historical StepRun.
// The Conductor definition never embeds an Asset ID or a caller-supplied URL.
func (h Handler) Execute(ctx context.Context, input Input) (finalize_task.Output, error) {
	if h.Jobs == nil || h.Steps == nil || h.Assets == nil || input.JobID == "" || input.StepKey == "" {
		return finalize_task.Output{}, errors.New("historical restore is not configured")
	}
	job, err := h.Jobs.Get(ctx, input.JobID)
	if err != nil {
		return finalize_task.Output{}, fmt.Errorf("load historical Job: %w", err)
	}
	if job.Origin != "legacy_import" || job.Status != jobdomain.JobRunning || job.ResumeGeneration <= 0 {
		return finalize_task.Output{}, errors.New("historical Job is not actively resuming")
	}
	step, err := h.Steps.GetByJobAndStepKey(ctx, job.ID, input.StepKey)
	if err != nil {
		return finalize_task.Output{}, fmt.Errorf("load historical StepRun: %w", err)
	}
	if step.Origin != "legacy_import" || step.Status != jobdomain.SRSucceeded || step.OutputAssetID == "" ||
		step.SourceCreatedAt == nil || step.SourceSHA256 == "" {
		return finalize_task.Output{}, errors.New("step is not a successful historical output")
	}
	asset, err := h.Assets.GetImportedGeneratedAsset(ctx, step.OutputAssetID)
	if err != nil {
		return finalize_task.Output{}, fmt.Errorf("verify imported output: %w", err)
	}
	if asset.ID != step.OutputAssetID || asset.OrganizationID != job.OrganizationID || asset.Scope != job.Scope ||
		asset.OwnerUserID != job.SubmittedBy || asset.AssetKind != step.AssetKind ||
		asset.SourceSystem != step.SourceSystem || asset.SourceRecordID != step.SourceRecordID ||
		asset.SourceSHA256 != step.SourceSHA256 || asset.SourceCreatedAt != step.SourceCreatedAt.UnixNano() ||
		asset.AccessURL == "" || asset.AccessExpiresAt <= time.Now().Add(15*time.Second).Unix() {
		return finalize_task.Output{}, errors.New("historical output no longer matches its frozen StepRun")
	}
	return finalize_task.Output{
		OutputResourceID: asset.ID,
		OutputResource: finalize_task.Resource{
			ID: asset.ID, AssetRef: asset.ID, URL: asset.AccessURL, AssetKind: asset.AssetKind,
		},
	}, nil
}
