package asset_kind

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"media_agent/media_materials/biz/dal"
	"media_agent/media_materials/biz/dal/model"
	"media_agent/media_materials/biz/dal/query"
	domain "media_agent/media_materials/biz/domain/asset_kind"
)

// Repo 实现 domain.Repo（PG，GORM Gen query）。
// 从事务 ctx 取连接（dal.FromContext），支持上层 RunInTx 透传。
type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, ak *domain.AssetKind) error {
	m := toModel(ak)
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	return query.Use(dal.FromContext(ctx, r.db)).AssetKind.WithContext(ctx).Create(m)
}

func (r *Repo) Get(ctx context.Context, name string) (*domain.AssetKind, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	m, err := Q.AssetKind.WithContext(ctx).Where(Q.AssetKind.Name.Eq(name)).First()
	if err != nil {
		return nil, err
	}
	return toDomain(m), nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.AssetKind, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.AssetKind.WithContext(ctx)
	if q.ResourceType != "" {
		tx = tx.Where(Q.AssetKind.ResourceType.Eq(q.ResourceType))
	}
	if q.MediaType != "" {
		tx = tx.Where(Q.AssetKind.MediaType.Eq(q.MediaType))
	}
	if q.Status != "" {
		tx = tx.Where(Q.AssetKind.Status.Eq(q.Status))
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
	ms, err := tx.Order(Q.AssetKind.Name).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.AssetKind, len(ms))
	for i := range ms {
		out[i] = toDomain(ms[i])
	}
	return out, int(total), nil
}

func (r *Repo) Update(ctx context.Context, ak *domain.AssetKind) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.AssetKind.WithContext(ctx).
		Where(Q.AssetKind.Name.Eq(ak.Name)).
		Updates(map[string]any{
			"description": ak.Description,
			"status":      string(ak.Status),
			"updated_at":  time.Now(),
		})
	return err
}

func (r *Repo) Delete(ctx context.Context, name string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.AssetKind.WithContext(ctx).Where(Q.AssetKind.Name.Eq(name)).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repo) BatchGet(ctx context.Context, names []string) ([]*domain.AssetKind, error) {
	if len(names) == 0 {
		return nil, nil
	}
	Q := query.Use(dal.FromContext(ctx, r.db))
	ms, err := Q.AssetKind.WithContext(ctx).Where(Q.AssetKind.Name.In(names...)).Find()
	if err != nil {
		return nil, err
	}
	out := make([]*domain.AssetKind, len(ms))
	for i := range ms {
		out[i] = toDomain(ms[i])
	}
	return out, nil
}

// —— 转换 ——

func toModel(ak *domain.AssetKind) *model.AssetKind {
	return &model.AssetKind{
		Name:         ak.Name,
		ResourceType: string(ak.ResourceType),
		MediaType:    string(ak.MediaType),
		SourceTable:  ak.SourceTable,
		Description:  ak.Description,
		Status:       string(ak.Status),
		CreatedAt:    ak.CreatedAt,
		UpdatedAt:    ak.UpdatedAt,
	}
}

func toDomain(m *model.AssetKind) *domain.AssetKind {
	return &domain.AssetKind{
		Name:         m.Name,
		ResourceType: domain.ResourceType(m.ResourceType),
		MediaType:    domain.MediaType(m.MediaType),
		SourceTable:  m.SourceTable,
		Description:  m.Description,
		Status:       domain.Status(m.Status),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// ErrNotFound 资源不存在。
var ErrNotFound = errors.New("asset_kind not found")

func (r *Repo) CountMaterialsByAssetKind(ctx context.Context, name string) (int64, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	return Q.Material.WithContext(ctx).Where(Q.Material.AssetKind.Eq(name)).Count()
}

func (r *Repo) CountGeneratedAssetsByAssetKind(ctx context.Context, name string) (int64, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	return Q.GeneratedAsset.WithContext(ctx).Where(Q.GeneratedAsset.AssetKind.Eq(name)).Count()
}
