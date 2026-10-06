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

// reviewRow is a scan-only mapping for the explicit locking/CAS queries below.
// It is not exposed outside the repository and never auto-migrates the schema.
type reviewRow struct {
	ID, JobID, StepRunID                                    string
	ConductorWorkflowID, ConductorTaskID, TaskReferenceName string
	Generation                                              int
	AcceptedRetryCount                                      *int
	Decision, DecidedBy, IdempotencyKey, Feedback           string
	SelectedIndex                                           *int
	CandidatesSnapshot, OutputJSON                          json.RawMessage
	RequestDigest, Status, ClaimToken                       string
}

func (row reviewRow) domain() *domain.ReviewDecision {
	return &domain.ReviewDecision{ID: row.ID, JobID: row.JobID, StepRunID: row.StepRunID,
		ConductorWorkflowID: row.ConductorWorkflowID, ConductorTaskID: row.ConductorTaskID, TaskReferenceName: row.TaskReferenceName,
		Generation: row.Generation, Decision: row.Decision, DecidedBy: row.DecidedBy, IdempotencyKey: row.IdempotencyKey,
		AcceptedRetryCount: row.AcceptedRetryCount,
		Feedback:           row.Feedback, SelectedIndex: row.SelectedIndex, CandidatesSnapshot: row.CandidatesSnapshot, OutputJSON: row.OutputJSON,
		RequestDigest: row.RequestDigest, Status: row.Status, ClaimToken: row.ClaimToken}
}

func (r *JobRepo) GetReviewByKey(ctx context.Context, stepRunID, key string) (*domain.ReviewDecision, error) {
	var row reviewRow
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Table("media_workflow.step_run_reviews").
		Where("step_run_id=? AND idempotency_key=?", stepRunID, key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.domain(), nil
}

func (r *JobRepo) GetReviewByTask(ctx context.Context, stepRunID, taskID string) (*domain.ReviewDecision, error) {
	var row reviewRow
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Table("media_workflow.step_run_reviews").Where("step_run_id=? AND conductor_task_id=?", stepRunID, taskID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.domain(), nil
}

func (r *JobRepo) LatestReviewsByJob(ctx context.Context, jobID string) ([]domain.ReviewDecision, error) {
	var rows []reviewRow
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Raw(`SELECT DISTINCT ON (step_run_id) * FROM media_workflow.step_run_reviews
		WHERE job_id=? ORDER BY step_run_id,created_at DESC,id DESC`, jobID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.ReviewDecision, 0, len(rows))
	for _, row := range rows {
		result = append(result, *row.domain())
	}
	return result, nil
}

func (r *JobRepo) AcceptReview(ctx context.Context, review domain.ReviewDecision) (*domain.ReviewDecision, bool, error) {
	if err := review.Validate(); err != nil {
		return nil, false, err
	}
	tx := dal.FromContext(ctx, r.db)
	if tx == r.db {
		return nil, false, errors.New("approval aggregate transaction required")
	}
	tx = tx.WithContext(ctx)
	var job model.Job
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", review.JobID).First(&job).Error; err != nil {
		return nil, false, err
	}
	var step model.StepRun
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", review.StepRunID).First(&step).Error; err != nil {
		return nil, false, err
	}
	if step.JobID != job.ID || job.SubmittedBy != review.DecidedBy {
		return nil, false, domain.ErrReviewConflict
	}
	existing, err := r.GetReviewByKey(ctx, review.StepRunID, review.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.RequestDigest != review.RequestDigest {
			return nil, false, domain.ErrReviewConflict
		}
		return existing, false, nil
	}
	var candidates []domain.Candidate
	if review.AcceptedRetryCount != nil && *review.AcceptedRetryCount != int(job.RetryCount) {
		return nil, false, domain.ErrReviewConflict
	}
	if job.ConductorWorkflowID != review.ConductorWorkflowID || domain.JobStatus(job.Status).IsTerminal() ||
		step.Status != string(domain.SRNeedConfirm) || json.Unmarshal(step.Candidates, &candidates) != nil || !review.MatchesCandidates(candidates) {
		return nil, false, domain.ErrReviewConflict
	}
	result := tx.Exec(`INSERT INTO media_workflow.step_run_reviews
		(id,job_id,step_run_id,conductor_workflow_id,conductor_task_id,task_reference_name,generation,
		 decision,decided_by,idempotency_key,selected_index,feedback,candidates_snapshot,output_json,request_digest,accepted_retry_count)
		VALUES (?,?,?,?,?,?,?,?,?,?,?, ?,?::jsonb,?::jsonb,?,?) ON CONFLICT DO NOTHING`,
		review.ID, review.JobID, review.StepRunID, review.ConductorWorkflowID, review.ConductorTaskID, review.TaskReferenceName,
		review.Generation, review.Decision, review.DecidedBy, review.IdempotencyKey, review.SelectedIndex, review.Feedback,
		string(review.CandidatesSnapshot), string(review.OutputJSON), review.RequestDigest, review.AcceptedRetryCount)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, false, domain.ErrReviewConflict
	}
	if err := tx.Model(&step).Updates(map[string]any{"status": string(domain.SRClaimed), "selected_index": review.SelectedIndex}).Error; err != nil {
		return nil, false, err
	}
	if err := tx.Exec(`UPDATE media_workflow.jobs SET status=CASE
		WHEN EXISTS (SELECT 1 FROM media_workflow.step_runs WHERE job_id=? AND status='need_confirm')
		THEN 'need_confirm' ELSE 'running' END WHERE id=?`, job.ID, job.ID).Error; err != nil {
		return nil, false, err
	}
	review.Status = "pending"
	return &review, true, nil
}

func (r *JobRepo) LockReviewProjection(ctx context.Context, jobID, stepRunID string) error {
	tx := dal.FromContext(ctx, r.db)
	if tx == r.db {
		return errors.New("approval projection transaction required")
	}
	var job model.Job
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", jobID).First(&job).Error; err != nil {
		return err
	}
	if stepRunID == "" {
		return nil
	}
	var step model.StepRun
	return tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND job_id=?", stepRunID, jobID).First(&step).Error
}

func (r *JobRepo) ClaimPendingReview(ctx context.Context) (*domain.ReviewDecision, error) {
	var row reviewRow
	err := r.db.WithContext(ctx).Raw(`UPDATE media_workflow.step_run_reviews AS r
		SET claim_token=?,claimed_at=now(),delivery_attempts=delivery_attempts+1,updated_at=now()
		WHERE r.id=(SELECT id FROM media_workflow.step_run_reviews
		WHERE status='pending' AND next_attempt_at<=now()
		AND (claimed_at IS NULL OR claimed_at<now()-interval '2 minutes')
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING r.*`, uuid.NewString()).Scan(&row).Error
	if err != nil || row.ID == "" {
		return nil, err
	}
	return row.domain(), nil
}

func (r *JobRepo) FinishReview(ctx context.Context, review domain.ReviewDecision, status, reasonCode string) error {
	if (status != "applied" && status != "failed") || review.ClaimToken == "" || !validReviewReasonCode(reasonCode) ||
		(status == "failed" && reasonCode == "") || (status == "applied" && reasonCode != "") {
		return errors.New("invalid approval delivery completion")
	}
	var appliedAt *time.Time
	if status == "applied" {
		now := time.Now().UTC()
		appliedAt = &now
	}
	result := r.db.WithContext(ctx).Exec(`UPDATE media_workflow.step_run_reviews
		SET status=?,claim_token=NULL,claimed_at=NULL,last_error=?,updated_at=now(),applied_at=?
		WHERE id=? AND status='pending' AND claim_token=?`, status, reasonCode, appliedAt, review.ID, review.ClaimToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrReviewConflict
	}
	return nil
}

func (r *JobRepo) ReleaseReviewClaim(ctx context.Context, review domain.ReviewDecision, reasonCode string) error {
	if review.ClaimToken == "" || reasonCode == "" || !validReviewReasonCode(reasonCode) {
		return errors.New("invalid approval delivery claim")
	}
	result := r.db.WithContext(ctx).Exec(`UPDATE media_workflow.step_run_reviews
		SET claim_token=NULL,claimed_at=NULL,next_attempt_at=now()+interval '10 seconds',last_error=?,updated_at=now()
		WHERE id=? AND status='pending' AND claim_token=?`, reasonCode, review.ID, review.ClaimToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrReviewConflict
	}
	return nil
}

func validReviewReasonCode(code string) bool {
	if len(code) > 64 {
		return false
	}
	for _, char := range code {
		if char != '_' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

var _ domain.ReviewRepo = (*JobRepo)(nil)
