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
	row := &model.WorkflowCompilation{
		ID: item.ID, WorkflowID: item.WorkflowID, WorkflowVersion: item.WorkflowVersion,
		ConductorName: item.ConductorName, ConductorVersion: item.ConductorVersion,
		IRJSON: item.IRJSON, DefinitionJSON: item.DefinitionJSON, DefinitionHash: item.DefinitionHash,
		Status: item.Status, ErrorMessage: item.ErrorMessage, CreatedAt: item.CreatedAt,
	}
	return dal.FromContext(ctx, r.db).WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "workflow_id"}, {Name: "workflow_version"}},
		DoUpdates: clause.AssignmentColumns([]string{"conductor_name", "conductor_version", "ir_json", "definition_json", "definition_hash", "status", "error_message"}),
	}).Create(row).Error
}

func (r *CompilationRepo) MarkRegistered(ctx context.Context, id string) error {
	now := time.Now()
	return dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.WorkflowCompilation{}).
		Where("id = ?", id).Updates(map[string]any{"status": "registered", "registered_at": now, "error_message": ""}).Error
}

func (r *CompilationRepo) MarkFailed(ctx context.Context, id, message string) error {
	return dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.WorkflowCompilation{}).
		Where("id = ?", id).Updates(map[string]any{"status": "failed", "error_message": message}).Error
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
