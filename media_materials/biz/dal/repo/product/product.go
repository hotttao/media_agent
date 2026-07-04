package product

import (
	"context"
	"time"

	"gorm.io/gorm"

	"media_agent/media_materials/biz/dal"
	"media_agent/media_materials/biz/dal/model"
	"media_agent/media_materials/biz/dal/query"
	domain "media_agent/media_materials/biz/domain/product"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, p *domain.Product) error {
	row := toModel(p)
	row.CreatedAt = time.Now()
	row.UpdatedAt = time.Now()
	return query.Use(dal.FromContext(ctx, r.db)).Product.WithContext(ctx).Create(row)
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.Product, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.Product.WithContext(ctx).
		Where(Q.Product.ID.Eq(id), Q.Product.DeletedAt.IsNull()).First()
	if err != nil {
		return nil, err
	}
	return toDomain(row), nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.Product, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.Product.WithContext(ctx).Where(Q.Product.DeletedAt.IsNull())
	if q.Scope != "" {
		tx = tx.Where(Q.Product.Scope.Eq(q.Scope))
	}
	if q.Name != "" {
		tx = tx.Where(Q.Product.Name.Like("%" + q.Name + "%"))
	}
	if q.Status != "" {
		tx = tx.Where(Q.Product.Status.Eq(q.Status))
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
	rows, err := tx.Order(Q.Product.UpdatedAt.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Product, len(rows))
	for i := range rows {
		out[i] = toDomain(rows[i])
	}
	return out, int(total), nil
}

func (r *Repo) Update(ctx context.Context, p *domain.Product) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.Product.WithContext(ctx).
		Where(Q.Product.ID.Eq(p.ID), Q.Product.DeletedAt.IsNull()).
		Updates(map[string]any{
			"name":            p.Name,
			"description":     p.Description,
			"target_audience": p.TargetAudience,
			"product_details": p.ProductDetails,
			"display_actions": p.DisplayActions,
			"status":          string(p.Status),
			"updated_by":      p.UpdatedBy,
			"updated_at":      time.Now(),
		})
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.Product.WithContext(ctx).
		Where(Q.Product.ID.Eq(id), Q.Product.DeletedAt.IsNull()).
		Updates(map[string]any{"deleted_at": time.Now()})
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func toModel(p *domain.Product) *model.Product {
	return &model.Product{
		ID: p.ID, Scope: p.Scope, Name: p.Name, Description: p.Description,
		TargetAudience: p.TargetAudience, ProductDetails: p.ProductDetails,
		DisplayActions: p.DisplayActions, Status: string(p.Status),
		CreatedBy: p.CreatedBy, UpdatedBy: p.UpdatedBy,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toDomain(row *model.Product) *domain.Product {
	return &domain.Product{
		ID: row.ID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		TargetAudience: row.TargetAudience, ProductDetails: row.ProductDetails,
		DisplayActions: row.DisplayActions, Status: domain.Status(row.Status),
		CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
