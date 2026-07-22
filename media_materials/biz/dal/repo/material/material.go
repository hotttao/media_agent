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
	return query.Use(dal.FromContext(ctx, r.db)).Material.WithContext(ctx).Create(row)
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.Material, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.Material.WithContext(ctx).
		Where(Q.Material.ID.Eq(id), Q.Material.DeletedAt.IsNull()).First()
	if err != nil {
		return nil, err
	}
	return toDomain(row), nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.Material, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.Material.WithContext(ctx).Where(Q.Material.DeletedAt.IsNull())
	if q.Scope != "" {
		tx = tx.Where(Q.Material.Scope.Eq(q.Scope))
	}
	if q.AssetKind != "" {
		tx = tx.Where(Q.Material.AssetKind.Eq(q.AssetKind))
	}
	if q.Owner != "" {
		tx = tx.Where(Q.Material.OwnerUserID.Eq(q.Owner))
	}
	if q.Visibility != "" {
		tx = tx.Where(Q.Material.Visibility.Eq(q.Visibility))
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
	rows, err := tx.Order(Q.Material.UpdatedAt.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Material, len(rows))
	for i := range rows {
		out[i] = toDomain(rows[i])
	}
	return out, int(total), nil
}

func (r *Repo) Update(ctx context.Context, m *domain.Material) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Material.WithContext(ctx).
		Where(Q.Material.ID.Eq(m.ID), Q.Material.DeletedAt.IsNull()).
		Updates(map[string]any{
			"name":       m.Name,
			"url":        m.URL,
			"content":    m.Content,
			"visibility": string(m.Visibility),
			"team_id":    m.TeamID,
			"updated_at": time.Now(),
		})
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.Material.WithContext(ctx).
		Where(Q.Material.ID.Eq(id), Q.Material.DeletedAt.IsNull()).
		Updates(map[string]any{"deleted_at": time.Now()})
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
		ID: m.ID, Scope: m.Scope, Name: m.Name, AssetKind: m.AssetKind,
		URL: m.URL, Content: m.Content, OwnerUserID: m.OwnerUserID,
		Visibility: string(m.Visibility), TeamID: m.TeamID,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func toDomain(row *model.Material) *domain.Material {
	return &domain.Material{
		ID: row.ID, Scope: row.Scope, Name: row.Name, AssetKind: row.AssetKind,
		URL: row.URL, Content: row.Content, OwnerUserID: row.OwnerUserID,
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
