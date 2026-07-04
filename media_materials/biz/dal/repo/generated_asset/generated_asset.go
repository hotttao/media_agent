package generated_asset

import (
	"context"
	"time"

	"gorm.io/gorm"

	"media_agent/media_materials/biz/dal"
	"media_agent/media_materials/biz/dal/model"
	"media_agent/media_materials/biz/dal/query"
	domain "media_agent/media_materials/biz/domain/generated_asset"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, a *domain.GeneratedAsset) error {
	row := toModel(a)
	row.CreatedAt = time.Now()
	row.UpdatedAt = time.Now()
	return query.Use(dal.FromContext(ctx, r.db)).GeneratedAsset.WithContext(ctx).Create(row)
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.GeneratedAsset, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.GeneratedAsset.WithContext(ctx).
		Where(Q.GeneratedAsset.ID.Eq(id), Q.GeneratedAsset.DeletedAt.IsNull()).First()
	if err != nil {
		return nil, err
	}
	return toDomain(row), nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.GeneratedAsset, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.GeneratedAsset.WithContext(ctx).Where(Q.GeneratedAsset.DeletedAt.IsNull())
	if q.Scope != "" {
		tx = tx.Where(Q.GeneratedAsset.Scope.Eq(q.Scope))
	}
	if q.AssetKind != "" {
		tx = tx.Where(Q.GeneratedAsset.AssetKind.Eq(q.AssetKind))
	}
	if q.Owner != "" {
		tx = tx.Where(Q.GeneratedAsset.OwnerUserID.Eq(q.Owner))
	}
	if q.JobID != "" {
		tx = tx.Where(Q.GeneratedAsset.JobID.Eq(q.JobID))
	}
	if q.SourceVipID != "" {
		tx = tx.Where(Q.GeneratedAsset.SourceVirtualIPID.Eq(q.SourceVipID))
	}
	if q.SourceProdID != "" {
		tx = tx.Where(Q.GeneratedAsset.SourceProductID.Eq(q.SourceProdID))
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
	rows, err := tx.Order(Q.GeneratedAsset.CreatedAt.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.GeneratedAsset, len(rows))
	for i := range rows {
		out[i] = toDomain(rows[i])
	}
	return out, int(total), nil
}

func (r *Repo) ListBySource(ctx context.Context, sourceKind, sourceID string) ([]*domain.GeneratedAsset, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.GeneratedAsset.WithContext(ctx).Where(Q.GeneratedAsset.DeletedAt.IsNull())
	if sourceKind == "product" {
		tx = tx.Where(Q.GeneratedAsset.SourceProductID.Eq(sourceID))
	} else {
		tx = tx.Where(Q.GeneratedAsset.SourceVirtualIPID.Eq(sourceID))
	}
	rows, err := tx.Order(Q.GeneratedAsset.CreatedAt.Desc()).Find()
	if err != nil {
		return nil, err
	}
	out := make([]*domain.GeneratedAsset, len(rows))
	for i := range rows {
		out[i] = toDomain(rows[i])
	}
	return out, nil
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.GeneratedAsset.WithContext(ctx).
		Where(Q.GeneratedAsset.ID.Eq(id), Q.GeneratedAsset.DeletedAt.IsNull()).
		Updates(map[string]any{"deleted_at": time.Now()})
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func toModel(a *domain.GeneratedAsset) *model.GeneratedAsset {
	return &model.GeneratedAsset{
		ID: a.ID, Scope: a.Scope, Name: a.Name, AssetKind: a.AssetKind,
		URL: a.URL, Content: a.Content, OwnerUserID: a.OwnerUserID,
		Visibility: string(a.Visibility), TeamID: a.TeamID,
		JobID: a.JobID, StepRunID: a.StepRunID, TemplateID: a.TemplateID,
		TemplateVersion: a.TemplateVersion, SourceVirtualIPID: a.SourceVirtualIPID,
		SourceProductID: a.SourceProductID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toDomain(row *model.GeneratedAsset) *domain.GeneratedAsset {
	return &domain.GeneratedAsset{
		ID: row.ID, Scope: row.Scope, Name: row.Name, AssetKind: row.AssetKind,
		URL: row.URL, Content: row.Content, OwnerUserID: row.OwnerUserID,
		Visibility: domain.Visibility(row.Visibility), TeamID: row.TeamID,
		JobID: row.JobID, StepRunID: row.StepRunID, TemplateID: row.TemplateID,
		TemplateVersion: row.TemplateVersion, SourceVirtualIPID: row.SourceVirtualIPID,
		SourceProductID: row.SourceProductID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
