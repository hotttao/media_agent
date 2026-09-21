package product

import (
	"context"
	"time"

	"gorm.io/gorm"

	"media_agent/media_materials/biz/dal"
	"media_agent/media_materials/biz/dal/model"
	domain "media_agent/media_materials/biz/domain/product"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, p *domain.Product) error {
	return dal.FromContext(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := toModel(p)
		row.CreatedAt, row.UpdatedAt = time.Now(), time.Now()
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		return replaceImages(tx, p)
	})
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.Product, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	var row model.Product
	if err := db.Where("id = ? AND deleted_at IS NULL", id).First(&row).Error; err != nil {
		return nil, err
	}
	p := toDomain(&row)
	return p, loadImages(db, p)
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.Product, int, error) {
	tx := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.Product{}).Where("deleted_at IS NULL")
	if q.OrganizationID != "" {
		tx = tx.Where("organization_id = ?", q.OrganizationID)
	}
	if q.Scope != "" {
		tx = tx.Where("scope = ?", q.Scope)
	}
	if q.Name != "" {
		tx = tx.Where("name ILIKE ?", "%"+q.Name+"%")
	}
	if q.TargetAudience != "" {
		tx = tx.Where("target_audience = ?", q.TargetAudience)
	}
	if q.Tag != "" {
		tx = tx.Where("tags @> ?::jsonb", `["`+q.Tag+`"]`)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
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
	var rows []model.Product
	err = tx.Order("updated_at DESC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Product, len(rows))
	for i := range rows {
		out[i] = toDomain(&rows[i])
		if err := loadImages(dal.FromContext(ctx, r.db).WithContext(ctx), out[i]); err != nil {
			return nil, 0, err
		}
	}
	return out, int(total), nil
}

func (r *Repo) Update(ctx context.Context, p *domain.Product) error {
	return dal.FromContext(ctx, r.db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Product{}).Where("id = ? AND organization_id = ? AND deleted_at IS NULL", p.ID, p.OrganizationID).
			Updates(map[string]any{
				"name": p.Name, "description": p.Description, "target_audience": p.TargetAudience,
				"product_details": p.ProductDetails, "display_actions": p.DisplayActions,
				"tags": p.Tags, "external_commerce": p.ExternalCommerce, "metadata": p.Metadata,
				"status": string(p.Status), "updated_by": p.UpdatedBy, "updated_at": time.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return replaceImages(tx, p)
	})
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	res := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.Product{}).
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

func toModel(p *domain.Product) *model.Product {
	return &model.Product{
		ID: p.ID, OrganizationID: p.OrganizationID, Scope: p.Scope, Name: p.Name, Description: p.Description,
		TargetAudience: p.TargetAudience, ProductDetails: p.ProductDetails,
		DisplayActions: p.DisplayActions, Tags: p.Tags, ExternalCommerce: p.ExternalCommerce, Metadata: p.Metadata,
		Status:    string(p.Status),
		CreatedBy: p.CreatedBy, UpdatedBy: p.UpdatedBy,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toDomain(row *model.Product) *domain.Product {
	return &domain.Product{
		ID: row.ID, OrganizationID: row.OrganizationID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		TargetAudience: row.TargetAudience, ProductDetails: row.ProductDetails,
		DisplayActions: row.DisplayActions, Tags: row.Tags, ExternalCommerce: row.ExternalCommerce, Metadata: row.Metadata,
		Status:    domain.Status(row.Status),
		CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func replaceImages(tx *gorm.DB, p *domain.Product) error {
	if err := tx.Where("product_id = ?", p.ID).Delete(&model.ProductImage{}).Error; err != nil {
		return err
	}
	if len(p.Images) == 0 {
		return nil
	}
	rows := make([]model.ProductImage, len(p.Images))
	now := time.Now()
	for i, image := range p.Images {
		rows[i] = model.ProductImage{ID: image.ID, OrganizationID: p.OrganizationID, ProductID: p.ID,
			ObjectPath: image.ObjectPath, IsMain: image.IsMain, SortOrder: image.SortOrder, CreatedAt: now, UpdatedAt: now}
	}
	return tx.Create(&rows).Error
}

func loadImages(db *gorm.DB, p *domain.Product) error {
	var rows []model.ProductImage
	if err := db.Where("product_id = ?", p.ID).Order("sort_order ASC").Find(&rows).Error; err != nil {
		return err
	}
	p.Images = make([]domain.Image, len(rows))
	for i, row := range rows {
		p.Images[i] = domain.Image{ID: row.ID, OrganizationID: row.OrganizationID, ProductID: row.ProductID,
			ObjectPath: row.ObjectPath, IsMain: row.IsMain, SortOrder: row.SortOrder, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	}
	return nil
}
