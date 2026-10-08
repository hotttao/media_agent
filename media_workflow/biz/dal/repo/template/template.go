package template

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
			AssetKind:    t.Inputs[i].AssetKind,
			ResourceType: t.Inputs[i].ResourceType,
			Required:     t.Inputs[i].Required,
			DisplayOrder: t.Inputs[i].DisplayOrder,
			CreatedAt:    time.Now(),
		}
		if err := Q.TemplateInput.WithContext(ctx).Create(in); err != nil {
			return err
		}
	}
	// version=1 快照入 template_versions（供 workflow validate GetVersion 查询）
	inputsJSON, _ := json.Marshal(t.Inputs)
	invocationJSON, _ := json.Marshal(t.InvocationMapping)
	tv := &model.TemplateVersion{
		ID:         "tv_" + row.ID + "_" + pad(t.Version),
		TemplateID: row.ID, Version: t.Version,
		Status:     string(t.Status),
		PromptBody: t.PromptBody, OutputAssetKind: t.OutputAssetKind,
		Platform: t.Platform, GenWay: t.GenWay, GenType: t.GenType,
		NeedsConfirm: t.NeedsConfirm, CandidateCount: t.CandidateCount,
		CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion,
		InvocationMapping: invocationJSON,
		InputsSnapshot:    inputsJSON, CreatedBy: t.CreatedBy, CreatedAt: time.Now(),
	}
	return dal.FromContext(ctx, r.db).WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).Create(tv).Error
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

func (r *Repo) GetForUpdate(ctx context.Context, id string) (*domain.Template, error) {
	var row model.Template
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", id).First(&row).Error; err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) GetVersion(ctx context.Context, id string, version int) (*domain.TemplateVersion, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.TemplateVersion.WithContext(ctx).
		Where(Q.TemplateVersion.TemplateID.Eq(id), Q.TemplateVersion.Version.Eq(version)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 兼容旧 Update 只保存前一个版本快照的记录；当前版本仍在主表中。
			current, currentErr := r.Get(ctx, id)
			if currentErr == nil && current.Version == version {
				return &domain.TemplateVersion{
					TemplateID: current.ID, Version: current.Version, Status: current.Status,
					PromptBody: current.PromptBody, OutputAssetKind: current.OutputAssetKind,
					Platform: current.Platform, GenWay: current.GenWay, GenType: current.GenType,
					NeedsConfirm: current.NeedsConfirm, CandidateCount: current.CandidateCount,
					CapabilityKey: current.CapabilityKey, CapabilityVersion: current.CapabilityVersion,
					InvocationMapping: current.InvocationMapping,
					Inputs:            current.Inputs, CreatedBy: current.CreatedBy, CreatedAt: current.CreatedAt,
				}, nil
			}
		}
		return nil, err
	}
	var inputs []domain.Slot
	if err := json.Unmarshal(row.InputsSnapshot, &inputs); err != nil {
		return nil, err
	}
	var invocation domain.InvocationMapping
	if err := json.Unmarshal(row.InvocationMapping, &invocation); err != nil {
		return nil, err
	}
	return &domain.TemplateVersion{
		TemplateID: row.TemplateID, Version: row.Version, Status: domain.Status(row.Status),
		PromptBody: row.PromptBody, OutputAssetKind: row.OutputAssetKind,
		Platform: row.Platform, GenWay: row.GenWay, GenType: row.GenType,
		NeedsConfirm: row.NeedsConfirm, CandidateCount: row.CandidateCount,
		CapabilityKey: row.CapabilityKey, CapabilityVersion: row.CapabilityVersion,
		InvocationMapping: invocation,
		Inputs:            inputs, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}, nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.Template, int, error) {
	conn := dal.FromContext(ctx, r.db).WithContext(ctx)
	tx := conn.Model(&model.Template{}).Where("deleted_at IS NULL")
	if q.EnforceAccess {
		acl := conn.Where("created_by = ? OR visibility = ?", q.CallerID, "public")
		if len(q.TeamIDs) > 0 {
			granted := conn.Model(&model.TemplateGrant{}).Select("template_id").Where("team_id IN ? AND permission IN ?", q.TeamIDs, []string{"viewer", "editor"})
			acl = acl.Or("visibility = ? AND id IN (?)", "team", granted)
		}
		tx = tx.Where(acl)
	}
	if q.EnforceBoundary {
		tx = tx.Where("(organization_id = '' AND created_by = ?) OR (organization_id <> '' AND organization_id IN ?)", q.CallerID, q.OrganizationIDs)
	}
	if q.Scope != "" {
		tx = tx.Where("scope = ?", q.Scope)
	}
	if q.Name != "" {
		tx = tx.Where("name LIKE ?", "%"+q.Name+"%")
	}
	if q.Visibility != "" {
		tx = tx.Where("visibility = ?", q.Visibility)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	}
	if q.Offset > 0 {
		tx = tx.Offset(q.Offset)
	}
	var rows []*model.Template
	if err := tx.Order("updated_at DESC").Order("id ASC").Find(&rows).Error; err != nil {
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
		invocationJSON, err := json.Marshal(snapshot.InvocationMapping)
		if err != nil {
			return err
		}
		tv := &model.TemplateVersion{
			ID:         "tv_" + snapshot.TemplateID + "_" + pad(snapshot.Version),
			TemplateID: snapshot.TemplateID, Version: snapshot.Version, Status: string(snapshot.Status),
			PromptBody: snapshot.PromptBody, OutputAssetKind: snapshot.OutputAssetKind,
			Platform: snapshot.Platform, GenWay: snapshot.GenWay, GenType: snapshot.GenType,
			NeedsConfirm: snapshot.NeedsConfirm, CandidateCount: snapshot.CandidateCount,
			CapabilityKey: snapshot.CapabilityKey, CapabilityVersion: snapshot.CapabilityVersion,
			InvocationMapping: invocationJSON,
			InputsSnapshot:    inputsJSON, CreatedBy: snapshot.CreatedBy, CreatedAt: time.Now(),
		}
		if err := dal.FromContext(ctx, r.db).WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).Create(tv).Error; err != nil {
			return err
		}
	}
	// 2. 更新 templates
	row := toModel(t)
	row.UpdatedAt = time.Now()
	if _, err := Q.Template.WithContext(ctx).
		Where(Q.Template.ID.Eq(t.ID), Q.Template.DeletedAt.IsNull()).
		Updates(map[string]any{
			"status": string(t.Status),
			"name":   row.Name, "description": row.Description, "prompt_body": row.PromptBody,
			"output_asset_kind": row.OutputAssetKind, "platform": row.Platform,
			"gen_way": row.GenWay, "gen_type": row.GenType,
			"needs_confirm": row.NeedsConfirm, "candidate_count": row.CandidateCount,
			"capability_key": row.CapabilityKey, "capability_version": row.CapabilityVersion,
			"invocation_mapping": row.InvocationMapping,
			"version":            row.Version, "updated_at": row.UpdatedAt,
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
			Name: t.Inputs[i].Name, AssetKind: t.Inputs[i].AssetKind, ResourceType: t.Inputs[i].ResourceType,
			Required: t.Inputs[i].Required, DisplayOrder: t.Inputs[i].DisplayOrder,
			CreatedAt: time.Now(),
		}
		if err := Q.TemplateInput.WithContext(ctx).Create(in); err != nil {
			return err
		}
	}
	// 当前版本也须立即冻结；Workflow 发布会按该版本查询 template_versions。
	inputsJSON, err := json.Marshal(t.Inputs)
	if err != nil {
		return err
	}
	invocationJSON, err := json.Marshal(t.InvocationMapping)
	if err != nil {
		return err
	}
	return dal.FromContext(ctx, r.db).WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).Create(&model.TemplateVersion{
		ID: "tv_" + t.ID + "_" + pad(t.Version), TemplateID: t.ID, Version: t.Version,
		Status:     string(t.Status),
		PromptBody: t.PromptBody, OutputAssetKind: t.OutputAssetKind,
		Platform: t.Platform, GenWay: t.GenWay, GenType: t.GenType,
		NeedsConfirm: t.NeedsConfirm, CandidateCount: t.CandidateCount,
		CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion,
		InvocationMapping: invocationJSON, InputsSnapshot: inputsJSON,
		CreatedBy: t.CreatedBy, CreatedAt: time.Now(),
	}).Error
}

func (r *Repo) UpdateStatus(ctx context.Context, id string, status domain.Status) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Template.WithContext(ctx).
		Where(Q.Template.ID.Eq(id), Q.Template.DeletedAt.IsNull()).
		Updates(map[string]any{"status": string(status), "updated_at": time.Now()})
	if err != nil {
		return err
	}
	if status == domain.StatusPublished {
		return dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.TemplateVersion{}).Where("template_id = ? AND version = (SELECT version FROM media_workflow.templates WHERE id = ?)", id, id).Update("status", "published").Error
	}
	return nil
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
	invocationJSON, _ := json.Marshal(t.InvocationMapping)
	return &model.Template{
		OrganizationID: t.OrganizationID,
		ID:             t.ID, Scope: t.Scope, Name: t.Name, Description: t.Description,
		PromptBody: t.PromptBody, OutputAssetKind: t.OutputAssetKind,
		Platform: t.Platform, GenWay: t.GenWay, GenType: t.GenType,
		NeedsConfirm: t.NeedsConfirm, CandidateCount: t.CandidateCount,
		CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion,
		InvocationMapping: invocationJSON,
		Status:            string(t.Status), Visibility: string(t.Visibility),
		Version: t.Version, CreatedBy: t.CreatedBy,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func toDomain(row *model.Template, inputs []*model.TemplateInput) *domain.Template {
	var invocation domain.InvocationMapping
	_ = json.Unmarshal(row.InvocationMapping, &invocation)
	t := &domain.Template{
		OrganizationID: row.OrganizationID,
		ID:             row.ID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		PromptBody: row.PromptBody, OutputAssetKind: row.OutputAssetKind,
		Platform: row.Platform, GenWay: row.GenWay, GenType: row.GenType,
		NeedsConfirm: row.NeedsConfirm, CandidateCount: row.CandidateCount,
		CapabilityKey: row.CapabilityKey, CapabilityVersion: row.CapabilityVersion,
		InvocationMapping: invocation,
		Status:            domain.Status(row.Status), Visibility: domain.Visibility(row.Visibility),
		Version: row.Version, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	for _, in := range inputs {
		t.Inputs = append(t.Inputs, domain.Slot{
			Name: in.Name, AssetKind: in.AssetKind, ResourceType: in.ResourceType,
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
