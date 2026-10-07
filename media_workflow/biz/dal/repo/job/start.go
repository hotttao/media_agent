package job

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/job"
)

type startRow struct {
	ID, JobID, WorkflowName, CompilationHash, CorrelationID string
	WorkflowVersion                                         int
	WorkflowInput, TaskDomains                              json.RawMessage
	PayloadDigest, Status, ClaimToken, ConductorWorkflowID  string
}

func (s startRow) domain() *domain.ExecutionStart {
	return &domain.ExecutionStart{ID: s.ID, JobID: s.JobID, WorkflowName: s.WorkflowName, CompilationHash: s.CompilationHash, CorrelationID: s.CorrelationID, WorkflowVersion: s.WorkflowVersion, WorkflowInput: s.WorkflowInput, TaskDomains: s.TaskDomains, PayloadDigest: s.PayloadDigest, Status: s.Status, ClaimToken: s.ClaimToken, ConductorWorkflowID: s.ConductorWorkflowID}
}
func (r *JobRepo) EnqueueStart(ctx context.Context, s domain.ExecutionStart) error {
	if err := s.Validate(); err != nil {
		return err
	}
	tx := dal.FromContext(ctx, r.db)
	if tx == r.db {
		return errors.New("execution start aggregate transaction required")
	}
	var job model.Job
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", s.JobID).First(&job).Error; err != nil {
		return err
	}
	if job.Origin != "native" || job.Status != "pending" || job.ConductorWorkflowID != "" {
		return domain.ErrStartConflict
	}
	if job.CompilationHash != "" && job.CompilationHash != s.CompilationHash {
		return domain.ErrStartConflict
	}
	if err := tx.WithContext(ctx).Exec(`INSERT INTO media_workflow.execution_starts
        (id,job_id,workflow_name,workflow_version,compilation_hash,correlation_id,workflow_input,task_domains,payload_digest)
        VALUES (?,?,?,?,?,?,?::jsonb,?::jsonb,?)`, s.ID, s.JobID, s.WorkflowName, s.WorkflowVersion, s.CompilationHash, s.CorrelationID, string(s.WorkflowInput), string(s.TaskDomains), s.PayloadDigest).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&job).Update("compilation_hash", s.CompilationHash).Error
}
func (r *JobRepo) ClaimStart(ctx context.Context) (*domain.ExecutionStart, error) {
	var row startRow
	err := r.db.WithContext(ctx).Raw(`UPDATE media_workflow.execution_starts AS s SET claim_token=?,claimed_at=now(),delivery_attempts=delivery_attempts+1,updated_at=now()
        WHERE s.id=(SELECT id FROM media_workflow.execution_starts WHERE status IN ('queued','uncertain','cancel_pending') AND next_attempt_at<=now()
        AND (claimed_at IS NULL OR claimed_at<now()-interval '2 minutes') ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING s.*`, uuid.NewString()).Scan(&row).Error
	if err != nil || row.ID == "" {
		return nil, err
	}
	return row.domain(), nil
}
func (r *JobRepo) MarkStartSent(ctx context.Context, s domain.ExecutionStart) error {
	if s.ClaimToken == "" {
		return domain.ErrStartConflict
	}
	result := r.db.WithContext(ctx).Exec(`UPDATE media_workflow.execution_starts AS s SET status='uncertain',updated_at=now()
        WHERE id=? AND status='queued' AND claim_token=? AND claimed_at>=now()-interval '2 minutes'
        AND EXISTS (SELECT 1 FROM media_workflow.jobs j WHERE j.id=s.job_id AND j.status='pending' AND j.origin='native')`, s.ID, s.ClaimToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrStartConflict
	}
	return nil
}
func (r *JobRepo) FinishStart(ctx context.Context, s domain.ExecutionStart, executionID string) error {
	if s.ClaimToken == "" || executionID == "" {
		return domain.ErrStartConflict
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job model.Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", s.JobID).First(&job).Error; err != nil {
			return err
		}
		if job.Origin != "native" || job.Status != "pending" || job.CompilationHash != s.CompilationHash || job.ConductorWorkflowID != "" {
			return domain.ErrStartConflict
		}
		result := tx.Exec(`UPDATE media_workflow.execution_starts SET status='started',conductor_workflow_id=?,claim_token=NULL,claimed_at=NULL,last_error='',updated_at=now()
            WHERE id=? AND job_id=? AND status='uncertain' AND claim_token=? AND claimed_at>=now()-interval '2 minutes' AND payload_digest=?`, executionID, s.ID, s.JobID, s.ClaimToken, s.PayloadDigest)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrStartConflict
		}
		return tx.Model(&job).Updates(map[string]any{"conductor_workflow_id": executionID, "status": "running", "started_at": time.Now().UTC()}).Error
	})
}
func (r *JobRepo) ReleaseStartClaim(ctx context.Context, s domain.ExecutionStart, code string) error {
	if s.ClaimToken == "" || code == "" || !validReviewReasonCode(code) {
		return errors.New("invalid execution start release")
	}
	result := r.db.WithContext(ctx).Exec(`UPDATE media_workflow.execution_starts SET claim_token=NULL,claimed_at=NULL,last_error=?,next_attempt_at=now()+interval '10 seconds',updated_at=now()
        WHERE id=? AND status IN ('queued','uncertain','cancel_pending') AND claim_token=? AND claimed_at>=now()-interval '2 minutes'`, code, s.ID, s.ClaimToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrStartConflict
	}
	return nil
}

var _ domain.ExecutionStartRepo = (*JobRepo)(nil)

func (r *JobRepo) CancelInitialStart(ctx context.Context, jobID string) (bool, error) {
	handled := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var j model.Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", jobID).First(&j).Error; err != nil {
			return err
		}
		var row startRow
		err := tx.Table("media_workflow.execution_starts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("job_id=?", jobID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		handled = true
		if j.Status == "cancelled" {
			return nil
		}
		if j.Origin != "native" {
			return domain.ErrStartConflict
		}
		if err := tx.Model(&j).Updates(map[string]any{"status": "cancelled", "failure_reason": "cancelled by user", "finished_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		switch row.Status {
		case "queued", "started":
			status := "cancelled"
			if row.Status == "started" {
				status = "cancel_pending"
			}
			return tx.Exec(`UPDATE media_workflow.execution_starts SET status=?,claim_token=NULL,claimed_at=NULL,last_error='',next_attempt_at=now(),updated_at=now() WHERE id=?`, status, row.ID).Error
		case "uncertain":
			return nil // An in-flight sender retains its lease; no new Start.
		default:
			return domain.ErrStartConflict
		}
	})
	return handled, err
}

// Called only after the dispatcher has observed an upstream terminal instance.
func (r *JobRepo) FinishCancelledStart(ctx context.Context, s domain.ExecutionStart, executionID string) error {
	if s.ClaimToken == "" || executionID == "" {
		return domain.ErrStartConflict
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var j model.Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", s.JobID).First(&j).Error; err != nil {
			return err
		}
		if j.Origin != "native" || j.Status != "cancelled" || j.CompilationHash != s.CompilationHash {
			return domain.ErrStartConflict
		}
		result := tx.Exec(`UPDATE media_workflow.execution_starts SET status='cancelled',conductor_workflow_id=?,claim_token=NULL,claimed_at=NULL,last_error='',updated_at=now()
            WHERE id=? AND job_id=? AND status IN ('uncertain','cancel_pending') AND payload_digest=? AND claim_token=? AND claimed_at>=now()-interval '2 minutes'
            AND (conductor_workflow_id IS NULL OR conductor_workflow_id=?)`, executionID, s.ID, s.JobID, s.PayloadDigest, s.ClaimToken, executionID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrStartConflict
		}
		return nil
	})
}

var _ domain.ExecutionStartCancellationRepo = (*JobRepo)(nil)
