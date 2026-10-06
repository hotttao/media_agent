package job

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"media_agent/media_workflow/biz/dal"
	domain "media_agent/media_workflow/biz/domain/job"
)

type submissionRow struct {
	SubmittedBy, OrganizationID, WorkspaceID, IdempotencyKey, RequestDigest, JobID string
}

func (r *JobRepo) GetSubmission(ctx context.Context, key domain.SubmissionKey, lock bool) (*domain.Submission, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	tx := dal.FromContext(ctx, r.db).WithContext(ctx)
	if lock {
		if dal.FromContext(ctx, r.db) == r.db {
			return nil, errors.New("submission aggregate transaction required")
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", key.LockID()).Error; err != nil {
			return nil, err
		}
	}
	var row submissionRow
	err := tx.Table("media_workflow.job_submissions").Where("submitted_by=? AND organization_id=? AND workspace_id=? AND idempotency_key=?", key.SubmittedBy, key.OrganizationID, key.WorkspaceID, key.IdempotencyKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &domain.Submission{SubmissionKey: key, RequestDigest: row.RequestDigest, JobID: row.JobID}, nil
}

func (r *JobRepo) SaveSubmission(ctx context.Context, s domain.Submission) error {
	if err := s.Validate(); err != nil {
		return err
	}
	tx := dal.FromContext(ctx, r.db)
	if tx == r.db {
		return errors.New("submission aggregate transaction required")
	}
	// Ownership and initial-launch existence are checked in the same snapshot
	// as acceptance. No external authorization or engine calls occur here.
	result := tx.WithContext(ctx).Exec(`INSERT INTO media_workflow.job_submissions
        (submitted_by,organization_id,workspace_id,idempotency_key,request_digest,job_id)
        SELECT ?,?,?,?,?,? WHERE EXISTS (
          SELECT 1 FROM media_workflow.jobs j JOIN media_workflow.execution_starts s ON s.job_id=j.id
          WHERE j.id=? AND j.submitted_by=? AND j.organization_id=? AND j.workspace_id=? AND j.origin='native'
        )`, s.SubmittedBy, s.OrganizationID, s.WorkspaceID, s.IdempotencyKey, s.RequestDigest, s.JobID, s.JobID, s.SubmittedBy, s.OrganizationID, s.WorkspaceID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrSubmissionConflict
	}
	return nil
}

var _ domain.SubmissionRepo = (*JobRepo)(nil)
