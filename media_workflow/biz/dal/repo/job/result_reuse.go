package job

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"media_agent/media_workflow/biz/dal"
	domain "media_agent/media_workflow/biz/domain/job"
)

type ResultReuseRepo struct{ db *gorm.DB }

func NewResultReuseRepo(db *gorm.DB) *ResultReuseRepo { return &ResultReuseRepo{db: db} }

type reuseRow struct {
	ID, OrganizationID, WorkspaceID, OwnerUserID, Scope, Fingerprint string
	SourceJobID, SourceStepRunID, SourceResultKey, AssetID           string
	ObjectPath, Content, AssetKind                                   string
	CreatedAt                                                        time.Time
}

func (r reuseRow) domain() domain.ReuseEvidence {
	return domain.ReuseEvidence{ID: r.ID, Boundary: domain.ReuseScope{OrganizationID: r.OrganizationID, WorkspaceID: r.WorkspaceID, OwnerUserID: r.OwnerUserID, Scope: r.Scope}, Fingerprint: r.Fingerprint, SourceJobID: r.SourceJobID, SourceStepRunID: r.SourceStepRunID, SourceResultKey: r.SourceResultKey, AssetID: r.AssetID, ObjectPath: r.ObjectPath, Content: r.Content, AssetKind: r.AssetKind, CreatedAt: r.CreatedAt}
}
func (r *ResultReuseRepo) Find(ctx context.Context, boundary domain.ReuseScope, fingerprint string) ([]domain.ReuseEvidence, error) {
	if !boundary.Valid() || fingerprint == "" {
		return nil, domain.ErrReuseEvidenceConflict
	}
	var rows []reuseRow
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Table("media_workflow.step_result_reuse").Where("organization_id=? AND workspace_id=? AND owner_user_id=? AND scope=? AND fingerprint=?", boundary.OrganizationID, boundary.WorkspaceID, boundary.OwnerUserID, boundary.Scope, fingerprint).Order("created_at DESC, id DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	results := make([]domain.ReuseEvidence, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.domain())
	}
	return results, nil
}
func (r *ResultReuseRepo) Save(ctx context.Context, evidence domain.ReuseEvidence) error {
	if err := evidence.Validate(); err != nil {
		return err
	}
	boundary := evidence.Boundary
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	result := db.Exec(`INSERT INTO media_workflow.step_result_reuse
 (id,organization_id,workspace_id,owner_user_id,scope,fingerprint,source_job_id,source_step_run_id,source_result_key,asset_id,object_path,content,asset_kind)
 SELECT ?,?,?,?,?,?,?,?,?,?,?,?,? FROM media_workflow.jobs j JOIN media_workflow.step_runs s ON s.job_id=j.id
 WHERE j.id=? AND s.id=? AND j.organization_id=? AND j.workspace_id=? AND j.submitted_by=? AND j.scope=? AND s.asset_kind=? AND s.origin='native' AND j.origin='native' AND j.status<>'cancelled'
 ON CONFLICT DO NOTHING`, evidence.ID, boundary.OrganizationID, boundary.WorkspaceID, boundary.OwnerUserID, boundary.Scope, evidence.Fingerprint, evidence.SourceJobID, evidence.SourceStepRunID, evidence.SourceResultKey, evidence.AssetID, evidence.ObjectPath, evidence.Content, evidence.AssetKind,
		evidence.SourceJobID, evidence.SourceStepRunID, boundary.OrganizationID, boundary.WorkspaceID, boundary.OwnerUserID, boundary.Scope, evidence.AssetKind)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var row reuseRow
	err := db.Table("media_workflow.step_result_reuse").Where("id=?", evidence.ID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrReuseEvidenceConflict
	}
	if err != nil {
		return err
	}
	if !row.domain().Same(evidence) {
		return domain.ErrReuseEvidenceConflict
	}
	return nil
}

var _ domain.ResultReuseRepo = (*ResultReuseRepo)(nil)
