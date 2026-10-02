package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/job"
)

func (r *JobRepo) GetByLegacySource(ctx context.Context, workspaceID, sourceSystem, sourceRecordID string) (*domain.Job, error) {
	var row model.Job
	err := r.db.WithContext(ctx).Where("origin = ? AND workspace_id = ? AND source_system = ? AND source_record_id = ?",
		"legacy_import", workspaceID, sourceSystem, sourceRecordID).First(&row).Error
	if err != nil {
		return nil, err
	}
	return jobToDomain(&row), nil
}

// SaveImported inserts the historical Job and all of its StepRuns as one
// transaction. This path does not start Conductor or pretend to be SubmitJob.
func (r *JobRepo) SaveImported(ctx context.Context, j *domain.Job, stepRuns []*domain.StepRun) error {
	if j == nil || j.Origin != "legacy_import" || j.Status != domain.JobAwaitingResume || len(stepRuns) == 0 {
		return errors.New("invalid imported Job aggregate")
	}
	input, err := json.Marshal(j.Input)
	if err != nil {
		return fmt.Errorf("encode imported Job input: %w", err)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := &model.Job{
			ID: j.ID, Origin: j.Origin, OrganizationID: j.OrganizationID, WorkspaceID: j.WorkspaceID,
			SourceSystem: &j.SourceSystem, SourceRecordID: &j.SourceRecordID, SourceDigest: &j.SourceDigest,
			ImportedAt: j.ImportedAt, Scope: j.Scope, WorkflowID: j.WorkflowID, WorkflowVersion: j.WorkflowVersion,
			Input: datatypes.JSON(input), Status: string(j.Status), RetryHistory: datatypes.JSON("[]"),
			SubmittedBy: j.SubmittedBy, SubmittedAt: j.SubmittedAt,
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		for _, sr := range stepRuns {
			resolved, err := json.Marshal(sr.ResolvedInputs)
			if err != nil {
				return fmt.Errorf("encode historical resolved inputs: %w", err)
			}
			candidates, err := json.Marshal(sr.Candidates)
			if err != nil {
				return fmt.Errorf("encode historical candidates: %w", err)
			}
			step := &model.StepRun{
				ID: sr.ID, JobID: j.ID, StepKey: sr.StepKey, StepOrder: sr.StepOrder,
				Origin: sr.Origin, SourceCreatedAt: sr.SourceCreatedAt,
				TemplateID: sr.TemplateID, TemplateVersion: sr.TemplateVersion,
				ApprovalPolicy: string(sr.ApprovalPolicy), Status: string(sr.Status),
				ResolvedInputs: datatypes.JSON(resolved), Candidates: datatypes.JSON(candidates),
				SelectedIndex: sr.SelectedIndex, OutputAssetID: sr.OutputAssetID,
				FinishedAt: sr.FinishedAt, Platform: sr.Platform, GenWay: sr.GenWay, GenType: sr.GenType,
				AssetKind: sr.AssetKind, NeedsConfirm: sr.NeedsConfirm, CandidateCount: sr.CandidateCount,
			}
			if sr.Origin == "legacy_import" {
				step.SourceSystem, step.SourceRecordID, step.SourceSHA256 = &sr.SourceSystem, &sr.SourceRecordID, &sr.SourceSHA256
			}
			if err := tx.Create(step).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
