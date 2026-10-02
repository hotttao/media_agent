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

func (r *Repo) GetByStepRun(ctx context.Context, stepRunID string) (*domain.GeneratedAsset, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.GeneratedAsset.WithContext(ctx).
		Where(Q.GeneratedAsset.StepRunID.Eq(stepRunID), Q.GeneratedAsset.DeletedAt.IsNull()).First()
	if err != nil {
		return nil, err
	}
	return toDomain(row), nil
}

func (r *Repo) GetByLegacySource(ctx context.Context, sourceSystem, sourceRecordID string) (*domain.GeneratedAsset, error) {
	var row model.GeneratedAsset
	err := dal.FromContext(ctx, r.db).WithContext(ctx).
		Where("source_system = ? AND source_record_id = ? AND deleted_at IS NULL", sourceSystem, sourceRecordID).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return toDomain(&row), nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.GeneratedAsset, int, error) {
	if q.CallerID == "" || len(q.OrganizationIDs) == 0 {
		return nil, 0, nil
	}
	tx := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.GeneratedAsset{}).
		Where("deleted_at IS NULL AND organization_id IN ?", q.OrganizationIDs)
	if q.Scope != "" {
		tx = tx.Where("scope = ?", q.Scope)
	}
	if q.AssetKind != "" {
		tx = tx.Where("asset_kind = ?", q.AssetKind)
	}
	if q.Owner != "" {
		tx = tx.Where("owner_user_id = ?", q.Owner)
	}
	if q.JobID != "" {
		tx = tx.Where("job_id = ?", q.JobID)
	}
	if q.SourceVipID != "" {
		tx = tx.Where("source_virtual_ip_id = ?", q.SourceVipID)
	}
	if q.SourceProdID != "" {
		tx = tx.Where("source_product_id = ?", q.SourceProdID)
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
	var rows []model.GeneratedAsset
	if err := tx.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]*domain.GeneratedAsset, len(rows))
	for i := range rows {
		out[i] = toDomain(&rows[i])
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
		ID: a.ID, OrganizationID: a.OrganizationID, Scope: a.Scope, Name: a.Name, AssetKind: a.AssetKind,
		URL: a.URL, ObjectPath: a.ObjectPath, Content: a.Content, OwnerUserID: a.OwnerUserID,
		Visibility: string(a.Visibility), TeamID: a.TeamID,
		JobID: optionalString(a.JobID), StepRunID: optionalString(a.StepRunID), TemplateID: optionalString(a.TemplateID),
		TemplateVersion: optionalInt(a.TemplateVersion), Origin: a.Origin,
		SourceSystem: optionalString(a.SourceSystem), SourceRecordID: optionalString(a.SourceRecordID), SourceSHA256: optionalString(a.SourceSHA256),
		SourceCreatedAt: a.SourceCreatedAt, ImportedAt: a.ImportedAt, SourceVirtualIPID: a.SourceVirtualIPID,
		SourceProductID: a.SourceProductID, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toDomain(row *model.GeneratedAsset) *domain.GeneratedAsset {
	return &domain.GeneratedAsset{
		ID: row.ID, OrganizationID: row.OrganizationID, Scope: row.Scope, Name: row.Name, AssetKind: row.AssetKind,
		URL: row.URL, ObjectPath: row.ObjectPath, Content: row.Content, OwnerUserID: row.OwnerUserID,
		Visibility: domain.Visibility(row.Visibility), TeamID: row.TeamID,
		JobID: valueString(row.JobID), StepRunID: valueString(row.StepRunID), TemplateID: valueString(row.TemplateID),
		TemplateVersion: valueInt(row.TemplateVersion), Origin: row.Origin,
		SourceSystem: valueString(row.SourceSystem), SourceRecordID: valueString(row.SourceRecordID), SourceSHA256: valueString(row.SourceSHA256),
		SourceCreatedAt: row.SourceCreatedAt, ImportedAt: row.ImportedAt, SourceVirtualIPID: row.SourceVirtualIPID,
		SourceProductID: row.SourceProductID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func valueString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func valueInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
