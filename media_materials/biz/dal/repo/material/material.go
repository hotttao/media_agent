package material

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_materials/biz/dal"
	"media_agent/media_materials/biz/dal/model"
	"media_agent/media_materials/biz/dal/query"
	domain "media_agent/media_materials/biz/domain/material"
)

// Repo 实现 domain.Repo（GORM Gen）。
type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, m *domain.Material) error {
	row := toModel(m)
	row.CreatedAt = time.Now()
	row.UpdatedAt = time.Now()
	return dal.FromContext(ctx, r.db).WithContext(ctx).Create(row).Error
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.Material, error) {
	var row model.Material
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&row).Error
	return toDomain(&row), err
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.Material, int, error) {
	tx := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.Material{}).Where("deleted_at IS NULL")
	if q.OrganizationID != "" {
		tx = tx.Where("organization_id = ?", q.OrganizationID)
	}
	if q.Scope != "" {
		tx = tx.Where("scope = ?", q.Scope)
	}
	if q.AssetKind != "" {
		tx = tx.Where("asset_kind = ?", q.AssetKind)
	}
	if q.Owner != "" {
		tx = tx.Where("owner_user_id = ?", q.Owner)
	}
	if q.Visibility != "" {
		tx = tx.Where("visibility = ?", q.Visibility)
	}
	if len(q.CallerTeamIDs) > 0 {
		tx = tx.Where("owner_user_id = ? OR visibility = 'public' OR (visibility = 'team' AND team_id IN ?)", q.CallerID, q.CallerTeamIDs)
	} else {
		tx = tx.Where("owner_user_id = ? OR visibility = 'public'", q.CallerID)
	}
	var total int64
	err := tx.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	}
	if q.Offset > 0 {
		tx = tx.Offset(q.Offset)
	}
	var rows []model.Material
	err = tx.Order("updated_at DESC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Material, len(rows))
	for i := range rows {
		out[i] = toDomain(&rows[i])
	}
	return out, int(total), nil
}

func (r *Repo) Update(ctx context.Context, m *domain.Material) error {
	return dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.Material{}).
		Where("id = ? AND organization_id = ? AND deleted_at IS NULL", m.ID, m.OrganizationID).
		Updates(map[string]any{
			"name": m.Name, "description": m.Description, "prompt": m.Prompt,
			"object_path": m.ObjectPath, "content": m.Content, "metadata": m.Metadata,
			"visibility": string(m.Visibility), "team_id": m.TeamID, "updated_at": time.Now(),
		}).Error
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	res := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.Material{}).
		Where("id = ? AND deleted_at IS NULL", id).Update("deleted_at", time.Now())
	err := res.Error
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func toModel(m *domain.Material) *model.Material {
	return &model.Material{
		ID: m.ID, OrganizationID: m.OrganizationID, Scope: m.Scope, Name: m.Name,
		Description: m.Description, Prompt: m.Prompt, AssetKind: m.AssetKind,
		ObjectPath: m.ObjectPath, Content: m.Content, Metadata: m.Metadata, OwnerUserID: m.OwnerUserID,
		Visibility: string(m.Visibility), TeamID: m.TeamID,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func toDomain(row *model.Material) *domain.Material {
	return &domain.Material{
		ID: row.ID, OrganizationID: row.OrganizationID, Scope: row.Scope, Name: row.Name,
		Description: row.Description, Prompt: row.Prompt, AssetKind: row.AssetKind,
		ObjectPath: row.ObjectPath, Content: row.Content, Metadata: row.Metadata, OwnerUserID: row.OwnerUserID,
		Visibility: domain.Visibility(row.Visibility), TeamID: row.TeamID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// —— BindingRepo ——

type BindingRepo struct{ db *gorm.DB }

func NewBindingRepo(db *gorm.DB) *BindingRepo { return &BindingRepo{db: db} }

func (r *BindingRepo) AddBinding(ctx context.Context, b *domain.Binding) error {
	row := &model.MaterialBinding{
		MaterialID: b.MaterialID,
		OwnerKind:  string(b.OwnerKind),
		OwnerID:    b.OwnerID,
		BoundBy:    b.BoundBy,
		BoundAt:    time.Now(),
	}
	return query.Use(dal.FromContext(ctx, r.db)).MaterialBinding.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).Create(row)
}

func (r *BindingRepo) RemoveBinding(ctx context.Context, materialID string, ownerKind domain.BindingOwnerKind, ownerID string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.MaterialBinding.WithContext(ctx).
		Where(Q.MaterialBinding.MaterialID.Eq(materialID),
			Q.MaterialBinding.OwnerKind.Eq(string(ownerKind)),
			Q.MaterialBinding.OwnerID.Eq(ownerID)).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *BindingRepo) ListByOwner(ctx context.Context, ownerKind domain.BindingOwnerKind, ownerID string) ([]*domain.Material, error) {
	db := dal.FromContext(ctx, r.db)
	var rows []model.Material
	err := db.Table("media_materials.materials").
		Joins("JOIN media_materials.material_bindings ON material_bindings.material_id = materials.id").
		Where("material_bindings.owner_kind = ? AND material_bindings.owner_id = ? AND material_bindings.bindings_end_at IS NULL AND materials.deleted_at IS NULL",
			string(ownerKind), ownerID).
		Order("materials.updated_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Material, len(rows))
	for i := range rows {
		out[i] = toDomain(&rows[i])
	}
	return out, nil
}

// EndBindings 失效 owner 的全部生效 binding（置 bindings_end_at = now）。
// owner（virtual_ip / product）软删时由 material service.EndBindingsForOwner 调用。
func (r *BindingRepo) EndBindings(ctx context.Context, ownerKind, ownerID string) error {
	db := dal.FromContext(ctx, r.db)
	return db.Table("media_materials.material_bindings").
		Where("owner_kind = ? AND owner_id = ? AND bindings_end_at IS NULL", ownerKind, ownerID).
		Update("bindings_end_at", time.Now()).Error
}
