package workflow

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

type CompilationRepo struct{ db *gorm.DB }

func NewCompilationRepo(db *gorm.DB) *CompilationRepo { return &CompilationRepo{db: db} }

func (r *CompilationRepo) Save(ctx context.Context, item *domain.Compilation) error {
	if item == nil || item.ID == "" || item.WorkflowID == "" || item.WorkflowVersion <= 0 || item.ConductorName == "" || item.ConductorVersion <= 0 || item.DefinitionHash == "" {
		return domain.ErrCompilationConflict
	}
	row := &model.WorkflowCompilation{
		ID: item.ID, WorkflowID: item.WorkflowID, WorkflowVersion: item.WorkflowVersion,
		ConductorName: item.ConductorName, ConductorVersion: item.ConductorVersion,
		IRJSON: item.IRJSON, DefinitionJSON: item.DefinitionJSON, DefinitionHash: item.DefinitionHash,
		Status: item.Status, ErrorMessage: item.ErrorMessage, CreatedAt: item.CreatedAt,
	}
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workflow_id"}, {Name: "workflow_version"}},
		DoNothing: true,
	}).Create(row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	// JSONB equality ignores transport formatting and object-key order while
	// retaining arrays, scalar types and every frozen field. No lifecycle state
	// is reset when a publisher repeats an already accepted compilation.
	var matches int64
	err := db.Model(&model.WorkflowCompilation{}).
		Where("id=? AND workflow_id=? AND workflow_version=? AND conductor_name=? AND conductor_version=? AND definition_hash=? AND ir_json=?::jsonb AND definition_json=?::jsonb",
			item.ID, item.WorkflowID, item.WorkflowVersion, item.ConductorName, item.ConductorVersion, item.DefinitionHash, string(item.IRJSON), string(item.DefinitionJSON)).Count(&matches).Error
	if err != nil {
		return err
	}
	if matches != 1 {
		return domain.ErrCompilationConflict
	}
	return nil
}

func (r *CompilationRepo) MarkRegistered(ctx context.Context, id string) error {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	result := db.Model(&model.WorkflowCompilation{}).
		Where("id = ? AND status <> 'registered'", id).Updates(map[string]any{"status": "registered", "registered_at": gorm.Expr("COALESCE(registered_at, ?)", time.Now()), "error_message": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	return r.registeredOrNotFound(ctx, id)
}

func (r *CompilationRepo) MarkFailed(ctx context.Context, id, message string) error {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	result := db.Model(&model.WorkflowCompilation{}).
		Where("id = ? AND status <> 'registered'", id).Updates(map[string]any{"status": "failed", "error_message": message})
	if result.Error != nil || result.RowsAffected == 1 {
		return result.Error
	}
	return r.registeredOrNotFound(ctx, id) // Late failure cannot regress success.
}

func (r *CompilationRepo) registeredOrNotFound(ctx context.Context, id string) error {
	var registered int64
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.WorkflowCompilation{}).Where("id=? AND status='registered'", id).Count(&registered).Error; err != nil {
		return err
	}
	if registered != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *CompilationRepo) GetRegistered(ctx context.Context, workflowID string, workflowVersion int) (*domain.Compilation, error) {
	var row model.WorkflowCompilation
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).
		Where("workflow_id = ? AND workflow_version = ? AND status = ?", workflowID, workflowVersion, "registered").
		First(&row).Error; err != nil {
		return nil, err
	}
	return &domain.Compilation{
		ID: row.ID, WorkflowID: row.WorkflowID, WorkflowVersion: row.WorkflowVersion,
		ConductorName: row.ConductorName, ConductorVersion: row.ConductorVersion,
		IRJSON: row.IRJSON, DefinitionJSON: row.DefinitionJSON, DefinitionHash: row.DefinitionHash,
		Status: row.Status, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt, RegisteredAt: row.RegisteredAt,
	}, nil
}
