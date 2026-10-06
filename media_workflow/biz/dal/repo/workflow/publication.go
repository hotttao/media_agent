package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

type publicationRow struct {
	ID, WorkflowID, CompilationID, RequestedBy string
	WorkflowVersion                            int
	DefinitionHash                             string
	DefinitionJSON, TaskDefinitionsJSON        json.RawMessage
	PayloadDigest, Status, ClaimToken          string
}

func (r publicationRow) domain() *domain.Publication {
	return &domain.Publication{ID: r.ID, WorkflowID: r.WorkflowID, CompilationID: r.CompilationID, RequestedBy: r.RequestedBy, WorkflowVersion: r.WorkflowVersion,
		DefinitionHash: r.DefinitionHash, DefinitionJSON: r.DefinitionJSON, TaskDefinitionsJSON: r.TaskDefinitionsJSON, PayloadDigest: r.PayloadDigest, Status: r.Status, ClaimToken: r.ClaimToken}
}

func (r *CompilationRepo) GetPublication(ctx context.Context, workflowID string, version int) (*domain.Publication, error) {
	var row publicationRow
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Table("media_workflow.workflow_publications").Where("workflow_id=? AND workflow_version=?", workflowID, version).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.domain(), nil
}

func (r *CompilationRepo) EnqueuePublication(ctx context.Context, c *domain.Compilation, p domain.Publication) (*domain.Publication, bool, error) {
	if err := p.Validate(); err != nil {
		return nil, false, err
	}
	if c == nil || c.ID != p.CompilationID || c.WorkflowID != p.WorkflowID || c.WorkflowVersion != p.WorkflowVersion || c.DefinitionHash != p.DefinitionHash {
		return nil, false, domain.ErrPublicationConflict
	}
	tx := dal.FromContext(ctx, r.db)
	if tx == r.db {
		return nil, false, errors.New("publication acceptance transaction required")
	}
	tx = tx.WithContext(ctx)
	var source model.Workflow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", p.WorkflowID).First(&source).Error; err != nil {
		return nil, false, err
	}
	if source.CreatedBy != p.RequestedBy || source.DeletedAt != nil {
		return nil, false, domain.ErrPublicationConflict
	}
	var sameDefinition bool
	if err := tx.Raw("SELECT ?::jsonb = ?::jsonb", string(c.DefinitionJSON), string(p.DefinitionJSON)).Scan(&sameDefinition).Error; err != nil {
		return nil, false, err
	}
	if !sameDefinition {
		return nil, false, domain.ErrPublicationConflict
	}
	previous, err := r.GetPublication(ctx, p.WorkflowID, p.WorkflowVersion)
	if err != nil {
		return nil, false, err
	}
	if previous != nil {
		if previous.PayloadDigest != p.PayloadDigest {
			return nil, false, domain.ErrPublicationConflict
		}
		if err := r.Save(ctx, c); err != nil {
			return nil, false, err
		}
		return previous, false, nil
	}
	if int(source.Version) != p.WorkflowVersion || source.Status != "draft" {
		return nil, false, domain.ErrPublicationConflict
	}
	if err := r.Save(ctx, c); err != nil {
		return nil, false, err
	}
	if err := tx.Exec(`INSERT INTO media_workflow.workflow_publications
        (id,workflow_id,workflow_version,compilation_id,requested_by,definition_hash,definition_json,task_definitions_json,payload_digest)
        VALUES (?,?,?,?,?,?,?::jsonb,?::jsonb,?)`, p.ID, p.WorkflowID, p.WorkflowVersion, p.CompilationID, p.RequestedBy, p.DefinitionHash, string(p.DefinitionJSON), string(p.TaskDefinitionsJSON), p.PayloadDigest).Error; err != nil {
		return nil, false, err
	}
	if err := tx.Model(&source).Updates(map[string]any{"status": "publishing", "updated_at": gorm.Expr("now()")}).Error; err != nil {
		return nil, false, err
	}
	p.Status, p.ClaimToken = "pending", ""
	return &p, true, nil
}

func (r *CompilationRepo) ClaimPublication(ctx context.Context) (*domain.Publication, error) {
	var row publicationRow
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Raw(`UPDATE media_workflow.workflow_publications AS p
        SET claim_token=?,claimed_at=now(),delivery_attempts=delivery_attempts+1,updated_at=now()
        WHERE p.id=(SELECT id FROM media_workflow.workflow_publications
        WHERE status='pending' AND next_attempt_at<=now()
          AND (claimed_at IS NULL OR claimed_at<now()-interval '2 minutes')
        ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING p.*`, uuid.NewString()).Scan(&row).Error
	if err != nil || row.ID == "" {
		return nil, err
	}
	return row.domain(), nil
}

var publicationErrorCode = regexp.MustCompile(`^[a-z0-9_]{0,64}$`)

func (r *CompilationRepo) ReleasePublication(ctx context.Context, p domain.Publication, code string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.ClaimToken == "" || !publicationErrorCode.MatchString(code) {
		return domain.ErrPublicationClaimStale
	}
	result := dal.FromContext(ctx, r.db).WithContext(ctx).Exec(`UPDATE media_workflow.workflow_publications
        SET claim_token=NULL,claimed_at=NULL,last_error=?,next_attempt_at=now()+interval '10 seconds',updated_at=now()
        WHERE id=? AND status='pending' AND payload_digest=? AND claim_token=?
          AND claimed_at>now()-interval '2 minutes'`, code, p.ID, p.PayloadDigest, p.ClaimToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrPublicationClaimStale
	}
	return nil
}

func (r *CompilationRepo) FinishPublication(ctx context.Context, p domain.Publication, status, code string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.ClaimToken == "" || !publicationErrorCode.MatchString(code) || status != "registered" && status != "failed" && status != "superseded" {
		return domain.ErrPublicationConflict
	}
	tx := dal.FromContext(ctx, r.db)
	if tx == r.db {
		return errors.New("publication completion transaction required")
	}
	tx = tx.WithContext(ctx)
	var source model.Workflow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", p.WorkflowID).First(&source).Error; err != nil {
		return err
	}
	var current publicationRow
	if err := tx.Table("media_workflow.workflow_publications").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id=? AND workflow_id=? AND status='pending' AND payload_digest=? AND claim_token=? AND claimed_at>now()-interval '2 minutes'", p.ID, p.WorkflowID, p.PayloadDigest, p.ClaimToken).Take(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrPublicationClaimStale
		}
		return err
	}
	active := source.DeletedAt == nil && source.CreatedBy == p.RequestedBy && int(source.Version) == p.WorkflowVersion && source.Status == "publishing"
	if !active {
		status, code = "superseded", "source_version_changed"
	} else {
		if status == "superseded" {
			return domain.ErrPublicationConflict
		}
		if status == "registered" {
			if err := r.MarkRegistered(ctx, p.CompilationID); err != nil {
				return err
			}
			if err := tx.Model(&source).Updates(map[string]any{"status": "published", "updated_at": gorm.Expr("now()")}).Error; err != nil {
				return err
			}
		} else {
			if err := r.MarkFailed(ctx, p.CompilationID, code); err != nil {
				return err
			}
			if err := tx.Model(&source).Updates(map[string]any{"status": "publish_failed", "updated_at": gorm.Expr("now()")}).Error; err != nil {
				return err
			}
		}
	}
	result := tx.Exec(`UPDATE media_workflow.workflow_publications
        SET status=?,claim_token=NULL,claimed_at=NULL,last_error=?,finished_at=now(),updated_at=now()
        WHERE id=? AND status='pending' AND payload_digest=? AND claim_token=?`, status, code, p.ID, p.PayloadDigest, p.ClaimToken)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrPublicationClaimStale
	}
	return nil
}

var _ domain.PublicationRepo = (*CompilationRepo)(nil)
