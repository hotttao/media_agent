package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

// This storage representation intentionally has no access, identity or engine fields.
type definitionSnapshot struct {
	Scope         string                    `json:"scope"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	OutputStepKey string                    `json:"output_step_key"`
	Steps         []domain.Step             `json:"steps"`
	InputSchema   []domain.InputSchemaEntry `json:"input_schema"`
}

func snapshotOf(w *domain.Workflow) definitionSnapshot {
	steps := append([]domain.Step{}, w.Steps...)
	for i := range steps {
		steps[i].SlotBindings = append([]domain.SlotBinding{}, steps[i].SlotBindings...)
		sort.Slice(steps[i].SlotBindings, func(a, b int) bool {
			x, y := steps[i].SlotBindings[a], steps[i].SlotBindings[b]
			if x.DisplayOrder != y.DisplayOrder {
				return x.DisplayOrder < y.DisplayOrder
			}
			return x.ToInput < y.ToInput
		})
	}
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].Order != steps[j].Order {
			return steps[i].Order < steps[j].Order
		}
		return steps[i].Key < steps[j].Key
	})
	schema := append([]domain.InputSchemaEntry{}, w.InputSchema...)
	sort.Slice(schema, func(i, j int) bool { return schema[i].Name < schema[j].Name })
	return definitionSnapshot{Scope: w.Scope, Name: w.Name, Description: w.Description, OutputStepKey: w.OutputStepKey, Steps: steps, InputSchema: schema}
}

func (r *Repo) saveDefinitionVersion(ctx context.Context, w *domain.Workflow) error {
	raw, err := json.Marshal(snapshotOf(w))
	if err != nil {
		return err
	}
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	row := &model.WorkflowVersion{WorkflowID: w.ID, Version: w.Version, DefinitionSnapshot: raw, CreatedAt: time.Now()}
	result := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "workflow_id"}, {Name: "version"}}, DoNothing: true}).Create(row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	var matches int64
	if err := db.Model(&model.WorkflowVersion{}).Where("workflow_id=? AND version=? AND definition_snapshot=?::jsonb", w.ID, w.Version, string(raw)).Count(&matches).Error; err != nil {
		return err
	}
	if matches != 1 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Repo) GetDefinitionVersion(ctx context.Context, id string, version int) (*domain.DefinitionVersion, error) {
	var row model.WorkflowVersion
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("workflow_id=? AND version=?", id, version).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.compiledDefinitionVersion(ctx, id, version)
	}
	if err != nil {
		return nil, err
	}
	var snapshot definitionSnapshot
	if err := json.Unmarshal(row.DefinitionSnapshot, &snapshot); err != nil {
		return nil, err
	}
	var registered int64
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.WorkflowCompilation{}).Where("workflow_id=? AND workflow_version=? AND status='registered'", id, version).Count(&registered).Error; err != nil {
		return nil, err
	}
	return &domain.DefinitionVersion{Source: "snapshot", Registered: registered > 0, Definition: &domain.Workflow{ID: id, Version: version, Scope: snapshot.Scope, Name: snapshot.Name, Description: snapshot.Description, OutputStepKey: snapshot.OutputStepKey, Steps: snapshot.Steps, InputSchema: snapshot.InputSchema, UpdatedAt: row.CreatedAt}}, nil
}
