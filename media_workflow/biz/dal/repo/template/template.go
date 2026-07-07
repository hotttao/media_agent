package template

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	"media_agent/media_workflow/biz/dal/query"
	domain "media_agent/media_workflow/biz/domain/template"
)

// Repo 实现 domain.Repo（GORM Gen）。
// Save/Update 多表操作由 service 包在 RunInTx 内调用，repo 透传 ctx。
type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, t *domain.Template) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row := toModel(t)
	row.CreatedAt = time.Now()
	row.UpdatedAt = time.Now()
	if err := Q.Template.WithContext(ctx).Create(row); err != nil {
		return err
	}
	t.ID = row.ID
	t.CreatedAt = row.CreatedAt
	// inputs
	for i := range t.Inputs {
		in := &model.TemplateInput{
			ID:           "tin_" + row.ID + "_" + pad(i),
			TemplateID:   row.ID,
			Version:      row.Version,
			Name:         t.Inputs[i].Name,
			ResourceType: t.Inputs[i].ResourceType,
			Required:     t.Inputs[i].Required,
			DisplayOrder: t.Inputs[i].DisplayOrder,
			CreatedAt:    time.Now(),
		}
		if err := Q.TemplateInput.WithContext(ctx).Create(in); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.Template, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.Template.WithContext(ctx).
		Where(Q.Template.ID.Eq(id), Q.Template.DeletedAt.IsNull()).First()
	if err != nil {
		return nil, err
	}
	inputs, err := Q.TemplateInput.WithContext(ctx).
		Where(Q.TemplateInput.TemplateID.Eq(id), Q.TemplateInput.Version.Eq(row.Version)).
		Order(Q.TemplateInput.DisplayOrder).Find()
	if err != nil {
		return nil, err
	}
	return toDomain(row, inputs), nil
}

func (r *Repo) GetVersion(ctx context.Context, id string, version int) (*domain.TemplateVersion, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.TemplateVersion.WithContext(ctx).
		Where(Q.TemplateVersion.TemplateID.Eq(id), Q.TemplateVersion.Version.Eq(version)).First()
	if err != nil {
		return nil, err
	}
	var inputs []domain.Slot
	if err := json.Unmarshal(row.InputsSnapshot, &inputs); err != nil {
		return nil, err
	}
	return &domain.TemplateVersion{
		TemplateID: row.TemplateID, Version: row.Version,
		PromptBody: row.PromptBody, OutputAssetKind: row.OutputAssetKind,
		Platform: row.Platform, GenWay: row.GenWay, GenType: row.GenType,
		NeedsConfirm: row.NeedsConfirm, CandidateCount: row.CandidateCount,
		Inputs: inputs, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}, nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.Template, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.Template.WithContext(ctx).Where(Q.Template.DeletedAt.IsNull())
	if q.Scope != "" {
		tx = tx.Where(Q.Template.Scope.Eq(q.Scope))
	}
	if q.Name != "" {
		tx = tx.Where(Q.Template.Name.Like("%" + q.Name + "%"))
	}
	if q.Visibility != "" {
		tx = tx.Where(Q.Template.Visibility.Eq(q.Visibility))
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
	rows, err := tx.Order(Q.Template.UpdatedAt.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Template, len(rows))
	for i := range rows {
		out[i] = toDomain(rows[i], nil) // list 不带 inputs（按需 Get）
	}
	return out, int(total), nil
}

// Update 整体覆盖：先写 template_versions 快照，再更新 templates + 重建 inputs。
// service 包在 RunInTx 内调用。
func (r *Repo) Update(ctx context.Context, t *domain.Template, snapshot *domain.TemplateVersion) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	// 1. 旧版本入 template_versions
	if snapshot != nil {
		inputsJSON, err := json.Marshal(snapshot.Inputs)
		if err != nil {
			return err
		}
		tv := &model.TemplateVersion{
			ID:         "tv_" + snapshot.TemplateID + "_" + pad(snapshot.Version),
			TemplateID: snapshot.TemplateID, Version: snapshot.Version,
			PromptBody: snapshot.PromptBody, OutputAssetKind: snapshot.OutputAssetKind,
			Platform: snapshot.Platform, GenWay: snapshot.GenWay, GenType: snapshot.GenType,
			NeedsConfirm: snapshot.NeedsConfirm, CandidateCount: snapshot.CandidateCount,
			InputsSnapshot: inputsJSON, CreatedBy: snapshot.CreatedBy, CreatedAt: time.Now(),
		}
		if err := Q.TemplateVersion.WithContext(ctx).Create(tv); err != nil {
			return err
		}
	}
	// 2. 更新 templates
	row := toModel(t)
	row.UpdatedAt = time.Now()
	if _, err := Q.Template.WithContext(ctx).
		Where(Q.Template.ID.Eq(t.ID), Q.Template.DeletedAt.IsNull()).
		Updates(map[string]any{
			"name": row.Name, "description": row.Description, "prompt_body": row.PromptBody,
			"output_asset_kind": row.OutputAssetKind, "platform": row.Platform,
			"gen_way": row.GenWay, "gen_type": row.GenType,
			"needs_confirm": row.NeedsConfirm, "candidate_count": row.CandidateCount,
			"version": row.Version, "updated_at": row.UpdatedAt,
		}); err != nil {
		return err
	}
	// 3. 重建 inputs（删旧 + 建新）
	if _, err := Q.TemplateInput.WithContext(ctx).
		Where(Q.TemplateInput.TemplateID.Eq(t.ID), Q.TemplateInput.Version.Eq(t.Version-1)).Delete(); err != nil {
		return err
	}
	for i := range t.Inputs {
		in := &model.TemplateInput{
			ID:         "tin_" + t.ID + "_" + pad(i),
			TemplateID: t.ID, Version: t.Version,
			Name: t.Inputs[i].Name, ResourceType: t.Inputs[i].ResourceType,
			Required: t.Inputs[i].Required, DisplayOrder: t.Inputs[i].DisplayOrder,
			CreatedAt: time.Now(),
		}
		if err := Q.TemplateInput.WithContext(ctx).Create(in); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) UpdateStatus(ctx context.Context, id string, status domain.Status) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Template.WithContext(ctx).
		Where(Q.Template.ID.Eq(id), Q.Template.DeletedAt.IsNull()).
		Updates(map[string]any{"status": string(status), "updated_at": time.Now()})
	return err
}

func (r *Repo) UpdateVisibility(ctx context.Context, id string, visibility domain.Visibility) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Template.WithContext(ctx).
		Where(Q.Template.ID.Eq(id), Q.Template.DeletedAt.IsNull()).
		Updates(map[string]any{"visibility": string(visibility), "updated_at": time.Now()})
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.Template.WithContext(ctx).
		Where(Q.Template.ID.Eq(id), Q.Template.DeletedAt.IsNull()).
		Updates(map[string]any{"deleted_at": time.Now()})
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// —— 转换 ——

func toModel(t *domain.Template) *model.Template {
	return &model.Template{
		ID: t.ID, Scope: t.Scope, Name: t.Name, Description: t.Description,
		PromptBody: t.PromptBody, OutputAssetKind: t.OutputAssetKind,
		Platform: t.Platform, GenWay: t.GenWay, GenType: t.GenType,
		NeedsConfirm: t.NeedsConfirm, CandidateCount: t.CandidateCount,
		Status: string(t.Status), Visibility: string(t.Visibility),
		Version: t.Version, CreatedBy: t.CreatedBy,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func toDomain(row *model.Template, inputs []*model.TemplateInput) *domain.Template {
	t := &domain.Template{
		ID: row.ID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		PromptBody: row.PromptBody, OutputAssetKind: row.OutputAssetKind,
		Platform: row.Platform, GenWay: row.GenWay, GenType: row.GenType,
		NeedsConfirm: row.NeedsConfirm, CandidateCount: row.CandidateCount,
		Status: domain.Status(row.Status), Visibility: domain.Visibility(row.Visibility),
		Version: row.Version, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	for _, in := range inputs {
		t.Inputs = append(t.Inputs, domain.Slot{
			Name: in.Name, ResourceType: in.ResourceType,
			Required: in.Required, DisplayOrder: in.DisplayOrder,
		})
	}
	return t
}

// pad 简易序号填充（input id 用）。
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

// ErrNotFound 供 service 判断。
var ErrNotFound = errors.New("template not found")
