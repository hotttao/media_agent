package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/job"
)

func (r *JobRepo) BeginResume(ctx context.Context, start domain.ResumeStart, input domain.JobInput) (bool, error) {
	if start.ID == "" || start.JobID == "" || start.Generation <= 0 || start.IdempotencyKey == "" ||
		start.CorrelationID == "" || !json.Valid(start.DefinitionJSON) || !json.Valid(start.TaskDomainsJSON) || !json.Valid(start.WorkflowInputJSON) {
		return false, errors.New("invalid historical resume command")
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return false, err
	}
	created := false
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job model.Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", start.JobID).First(&job).Error; err != nil {
			return err
		}
		if job.Origin != "legacy_import" {
			return domain.ErrResumeConflict
		}
		if job.ResumeIdempotencyKey == start.IdempotencyKey && job.ResumeGeneration == start.Generation {
			return nil
		}
		if job.Status != string(domain.JobAwaitingResume) || job.ResumeGeneration+1 != start.Generation {
			return domain.ErrResumeConflict
		}
		if err := tx.Model(&job).Updates(map[string]any{
			"input": inputJSON, "status": string(domain.JobRunning), "resume_generation": start.Generation,
			"resume_idempotency_key": start.IdempotencyKey, "started_at": time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO media_workflow.resume_outbox
			(id,job_id,resume_generation,idempotency_key,correlation_id,definition,task_domains,workflow_input)
			VALUES (?,?,?,?,? ,?::jsonb,?::jsonb,?::jsonb)`, start.ID, start.JobID, start.Generation,
			start.IdempotencyKey, start.CorrelationID, string(start.DefinitionJSON),
			string(start.TaskDomainsJSON), string(start.WorkflowInputJSON)).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

func (r *JobRepo) ClaimPendingResume(ctx context.Context) (*domain.ResumeStart, error) {
	var row struct {
		ID               string
		JobID            string
		ResumeGeneration int
		IdempotencyKey   string
		CorrelationID    string
		Definition       json.RawMessage
		TaskDomains      json.RawMessage
		WorkflowInput    json.RawMessage
	}
	err := r.db.WithContext(ctx).Raw(`
		UPDATE media_workflow.resume_outbox AS o
		SET claimed_at=now(), attempts=attempts+1, updated_at=now()
		WHERE o.id = (
			SELECT id FROM media_workflow.resume_outbox
			WHERE status='pending' AND next_attempt_at <= now()
			  AND (claimed_at IS NULL OR claimed_at < now() - interval '2 minutes')
			ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
		)
		RETURNING o.id,o.job_id,o.resume_generation,o.idempotency_key,o.correlation_id,
			o.definition,o.task_domains,o.workflow_input`).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, nil
	}
	return &domain.ResumeStart{ID: row.ID, JobID: row.JobID, Generation: row.ResumeGeneration,
		IdempotencyKey: row.IdempotencyKey, CorrelationID: row.CorrelationID,
		DefinitionJSON: row.Definition, TaskDomainsJSON: row.TaskDomains, WorkflowInputJSON: row.WorkflowInput}, nil
}

func (r *JobRepo) MarkResumeStarted(ctx context.Context, start domain.ResumeStart, executionID, definitionHash string) error {
	if executionID == "" || definitionHash == "" {
		return errors.New("resume execution ID and definition hash are required")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Exec(`UPDATE media_workflow.jobs SET conductor_workflow_id=?, compilation_hash=?
			WHERE id=? AND origin='legacy_import' AND status='running' AND resume_generation=?
			  AND (conductor_workflow_id IS NULL OR conductor_workflow_id=? OR conductor_workflow_id='')`,
			executionID, definitionHash, start.JobID, start.Generation, executionID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("resume Job changed before Conductor start was recorded")
		}
		result = tx.Exec(`UPDATE media_workflow.resume_outbox
			SET status='started', conductor_workflow_id=?, claimed_at=NULL, updated_at=now()
			WHERE id=? AND job_id=? AND resume_generation=? AND status='pending'`,
			executionID, start.ID, start.JobID, start.Generation)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("resume outbox row changed before Conductor start was recorded")
		}
		return nil
	})
}

func (r *JobRepo) ReleaseResumeClaim(ctx context.Context, startID, reason string) error {
	if len(reason) > 1000 {
		reason = reason[:1000]
	}
	return r.db.WithContext(ctx).Exec(`UPDATE media_workflow.resume_outbox
		SET claimed_at=NULL, next_attempt_at=now()+interval '10 seconds', last_error=?, updated_at=now()
		WHERE id=? AND status='pending'`, reason, startID).Error
}
