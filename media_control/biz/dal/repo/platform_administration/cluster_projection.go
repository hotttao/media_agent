package platform_administration

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/platform_administration"
)

func (r *Repository) UpsertCluster(ctx context.Context, value *domain.ClusterProjection) error {
	row := &model.ClusterProjection{ClusterID: value.ClusterID, Status: string(value.Status), Capacity: value.CapacityJSON, ObservedAt: value.ObservedAt}
	return dal.FromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cluster_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "capacity", "observed_at"}),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "EXCLUDED.observed_at > cluster_projections.observed_at"}}},
	}).Create(row).Error
}
func (r *Repository) ListClusters(ctx context.Context) ([]domain.ClusterProjection, error) {
	var rows []model.ClusterProjection
	if err := dal.FromContext(ctx, r.db).Order("observed_at DESC, cluster_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.ClusterProjection, 0, len(rows))
	for _, row := range rows {
		result = append(result, toClusterDomain(row))
	}
	return result, nil
}
func (r *Repository) GetCluster(ctx context.Context, clusterID string) (*domain.ClusterProjection, error) {
	var row model.ClusterProjection
	err := dal.FromContext(ctx, r.db).Where("cluster_id = ?", clusterID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	value := toClusterDomain(row)
	return &value, nil
}
func toClusterDomain(row model.ClusterProjection) domain.ClusterProjection {
	return domain.ClusterProjection{ClusterID: row.ClusterID, Status: domain.ClusterStatus(row.Status), CapacityJSON: row.Capacity, ObservedAt: row.ObservedAt}
}
