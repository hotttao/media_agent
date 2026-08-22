package platform_administration

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/platform_administration"
)

func (r *Repository) UpsertStatistic(ctx context.Context, value *domain.PlatformStatistic) error {
	row := &model.PlatformStatistic{MetricKey: value.MetricKey, PeriodStart: value.PeriodStart, Dimensions: value.DimensionsJSON, DimensionsHash: value.DimensionsHash, Value: value.Value, RefreshedAt: value.RefreshedAt}
	return dal.FromContext(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "metric_key"}, {Name: "period_start"}, {Name: "dimensions_hash"}},
		DoUpdates: clause.AssignmentColumns([]string{"dimensions", "value", "refreshed_at"}),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "EXCLUDED.refreshed_at > platform_statistics.refreshed_at"}}},
	}).Create(row).Error
}
func (r *Repository) GetStatistic(ctx context.Context, metricKey string, periodStart time.Time, dimensionsHash string) (*domain.PlatformStatistic, error) {
	var row model.PlatformStatistic
	err := dal.FromContext(ctx, r.db).Where("metric_key = ? AND period_start = ? AND dimensions_hash = ?", metricKey, periodStart, dimensionsHash).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	value := toStatisticDomain(row)
	return &value, nil
}
func (r *Repository) ListStatistics(ctx context.Context, filter domain.StatisticFilter) ([]domain.PlatformStatistic, error) {
	query := dal.FromContext(ctx, r.db)
	if filter.MetricKey != "" {
		query = query.Where("metric_key = ?", filter.MetricKey)
	}
	if !filter.PeriodFrom.IsZero() {
		query = query.Where("period_start >= ?", filter.PeriodFrom)
	}
	if !filter.PeriodTo.IsZero() {
		query = query.Where("period_start <= ?", filter.PeriodTo)
	}
	var rows []model.PlatformStatistic
	if err := query.Order("period_start DESC, metric_key ASC, dimensions_hash ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.PlatformStatistic, 0, len(rows))
	for _, row := range rows {
		result = append(result, toStatisticDomain(row))
	}
	return result, nil
}
func toStatisticDomain(row model.PlatformStatistic) domain.PlatformStatistic {
	return domain.PlatformStatistic{MetricKey: row.MetricKey, PeriodStart: row.PeriodStart, DimensionsJSON: row.Dimensions, DimensionsHash: row.DimensionsHash, Value: row.Value, RefreshedAt: row.RefreshedAt}
}
