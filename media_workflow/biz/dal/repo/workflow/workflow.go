package workflow

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	"media_agent/media_workflow/biz/dal/query"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, w *domain.Workflow) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row := toModel(w)
	row.CreatedAt = time.Now()
	row.UpdatedAt = time.Now()
	if err := Q.Workflow.WithContext(ctx).Create(row); err != nil {
		return err
	}
	w.ID = row.ID
	w.CreatedAt = row.CreatedAt
	return r.saveSteps(ctx, Q, w)
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.Workflow, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.Workflow.WithContext(ctx).
		Where(Q.Workflow.ID.Eq(id), Q.Workflow.DeletedAt.IsNull()).First()
	if err != nil {
		return nil, err
	}
	var schema []domain.InputSchemaEntry
	_ = json.Unmarshal(row.InputSchema, &schema)
	w := &domain.Workflow{
		ID: row.ID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		OutputStepKey: row.OutputStepKey, Status: domain.Status(row.Status),
		Visibility: domain.Visibility(row.Visibility), Version: row.Version,
		InputSchema: schema, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	steps, err := Q.WorkflowStep.WithContext(ctx).
		Where(Q.WorkflowStep.WorkflowID.Eq(id), Q.WorkflowStep.Version.Eq(row.Version)).
		Order(Q.WorkflowStep.StepOrder).Find()
	if err != nil {
		return nil, err
	}
	for _, s := range steps {
		bindings, err := Q.StepBinding.WithContext(ctx).
			Where(Q.StepBinding.StepID.Eq(s.ID)).Order(Q.StepBinding.DisplayOrder).Find()
		if err != nil {
			return nil, err
		}
		w.Steps = append(w.Steps, stepToDomain(s, bindings))
	}
	return w, nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.WorkflowSummary, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.Workflow.WithContext(ctx).Where(Q.Workflow.DeletedAt.IsNull())
	if q.Scope != "" {
		tx = tx.Where(Q.Workflow.Scope.Eq(q.Scope))
	}
	if q.Name != "" {
		tx = tx.Where(Q.Workflow.Name.Like("%" + q.Name + "%"))
	}
	if q.Status != "" {
		tx = tx.Where(Q.Workflow.Status.Eq(q.Status))
	}
	if q.Visibility != "" {
		tx = tx.Where(Q.Workflow.Visibility.Eq(q.Visibility))
	}
	total, err := tx.Count()
	if err != nil {
		return nil, 0, err
	}
	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	}
	if q.Offset > 0 {
		tx = tx.Offset(q.Offset)
	}
	rows, err := tx.Order(Q.Workflow.UpdatedAt.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.WorkflowSummary, len(rows))
	for i := range rows {
		out[i] = summaryToDomain(rows[i])
	}
	return out, int(total), nil
}

// Update 整体覆盖：更新主表 + 删旧 steps/bindings 重建。service 包在 RunInTx 内调用。
func (r *Repo) Update(ctx context.Context, w *domain.Workflow) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	schemaJSON, _ := json.Marshal(w.InputSchema)
	if _, err := Q.Workflow.WithContext(ctx).
		Where(Q.Workflow.ID.Eq(w.ID), Q.Workflow.DeletedAt.IsNull()).
		Updates(map[string]any{
			"name": w.Name, "description": w.Description, "output_step_key": w.OutputStepKey,
			"version": w.Version, "input_schema": schemaJSON, "updated_at": time.Now(),
		}); err != nil {
		return err
	}
	// 删旧版本 steps（连带 bindings CASCADE）
	if _, err := Q.WorkflowStep.WithContext(ctx).
		Where(Q.WorkflowStep.WorkflowID.Eq(w.ID), Q.WorkflowStep.Version.Eq(w.Version-1)).Delete(); err != nil {
		return err
	}
	return r.saveSteps(ctx, Q, w)
}

func (r *Repo) UpdateStatus(ctx context.Context, id string, status domain.Status) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Workflow.WithContext(ctx).
		Where(Q.Workflow.ID.Eq(id), Q.Workflow.DeletedAt.IsNull()).
		Updates(map[string]any{"status": string(status), "updated_at": time.Now()})
	return err
}

func (r *Repo) UpdateVisibility(ctx context.Context, id string, visibility domain.Visibility) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Workflow.WithContext(ctx).
		Where(Q.Workflow.ID.Eq(id), Q.Workflow.DeletedAt.IsNull()).
		Updates(map[string]any{"visibility": string(visibility), "updated_at": time.Now()})
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.Workflow.WithContext(ctx).
		Where(Q.Workflow.ID.Eq(id), Q.Workflow.DeletedAt.IsNull()).
		Updates(map[string]any{"deleted_at": time.Now()})
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// saveSteps 写 steps + bindings（Save/Update 共用）。
func (r *Repo) saveSteps(ctx context.Context, Q *query.Query, w *domain.Workflow) error {
	for i := range w.Steps {
		s := &w.Steps[i]
		stepRow := &model.WorkflowStep{
			ID:         "wfs_" + w.ID + "_" + pad(s.Order),
			WorkflowID: w.ID, Version: w.Version,
			StepKey: s.Key, StepName: s.Name, StepOrder: s.Order,
			TemplateID: s.TemplateID, TemplateVersion: s.TemplateVersion,
			ApprovalPolicy: string(s.ApprovalPolicy), ReusePolicy: string(s.ReusePolicy),
			CreatedAt: time.Now(),
		}
		if err := Q.WorkflowStep.WithContext(ctx).Create(stepRow); err != nil {
			return err
		}
		for j := range s.SlotBindings {
			b := &s.SlotBindings[j]
			bindRow := &model.StepBinding{
				ID:     "sbd_" + stepRow.ID + "_" + pad(j),
				StepID: stepRow.ID, ToInput: b.ToInput,
				Source: string(b.Source), Ref: strPtr(b.Ref), ConstValue: strPtr(b.ConstValue),
				DisplayOrder: b.DisplayOrder, CreatedAt: time.Now(),
			}
			if err := Q.StepBinding.WithContext(ctx).Create(bindRow); err != nil {
				return err
			}
		}
	}
	return nil
}

// —— 转换 ——

func toModel(w *domain.Workflow) *model.Workflow {
	schemaJSON, _ := json.Marshal(w.InputSchema)
	return &model.Workflow{
		ID: w.ID, Scope: w.Scope, Name: w.Name, Description: w.Description,
		OutputStepKey: w.OutputStepKey, Status: string(w.Status), Visibility: string(w.Visibility),
		Version: w.Version, InputSchema: schemaJSON, CreatedBy: w.CreatedBy,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}

func summaryToDomain(row *model.Workflow) *domain.WorkflowSummary {
	var schema []domain.InputSchemaEntry
	_ = json.Unmarshal(row.InputSchema, &schema)
	return &domain.WorkflowSummary{
		ID: row.ID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		OutputStepKey: row.OutputStepKey, Status: domain.Status(row.Status),
		Visibility: domain.Visibility(row.Visibility), Version: row.Version,
		InputSchema: schema, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func stepToDomain(s *model.WorkflowStep, bindings []*model.StepBinding) domain.Step {
	st := domain.Step{
		Key: s.StepKey, Name: s.StepName, Order: s.StepOrder,
		TemplateID: s.TemplateID, TemplateVersion: s.TemplateVersion,
		ApprovalPolicy: domain.ApprovalPolicy(s.ApprovalPolicy), ReusePolicy: domain.ReusePolicy(s.ReusePolicy),
	}
	for _, b := range bindings {
		st.SlotBindings = append(st.SlotBindings, domain.SlotBinding{
			ToInput: b.ToInput, Source: domain.SlotBindingSource(b.Source),
			Ref: strVal(b.Ref), ConstValue: strVal(b.ConstValue), DisplayOrder: b.DisplayOrder,
		})
	}
	return st
}

func pad(i int) string {
	if i < 10 {
		return "0" + itoa(i)
	}
	return itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// GrantRepo ——

type GrantRepo struct{ db *gorm.DB }

func NewGrantRepo(db *gorm.DB) *GrantRepo { return &GrantRepo{db: db} }

func (r *GrantRepo) AddGrant(ctx context.Context, g *domain.Grant) error {
	row := &model.WorkflowGrant{
		WorkflowID: g.WorkflowID, TeamID: g.TeamID,
		Permission: string(g.Permission), GrantedBy: g.GrantedBy, GrantedAt: time.Now(),
	}
	return query.Use(dal.FromContext(ctx, r.db)).WorkflowGrant.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "workflow_id"}, {Name: "team_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"permission", "granted_by", "granted_at"}),
		}).Create(row)
}

func (r *GrantRepo) RemoveGrant(ctx context.Context, workflowID, teamID string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.WorkflowGrant.WithContext(ctx).
		Where(Q.WorkflowGrant.WorkflowID.Eq(workflowID), Q.WorkflowGrant.TeamID.Eq(teamID)).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
