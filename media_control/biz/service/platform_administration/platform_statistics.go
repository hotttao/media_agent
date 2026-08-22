package platform_administration

import (
	"context"
	"errors"
	"time"

	domain "media_agent/media_control/biz/domain/platform_administration"
)

type StatisticsService struct{ repo domain.StatisticsRepository }

func NewStatisticsService(repo domain.StatisticsRepository) *StatisticsService {
	return &StatisticsService{repo: repo}
}
func (s *StatisticsService) Report(ctx context.Context, actor domain.Actor, metricKey string, periodStartMillis int64, dimensionsJSON, value string, refreshedAtMillis int64) (*domain.PlatformStatistic, error) {
	if err := actor.ValidateSystem(); err != nil {
		return nil, err
	}
	periodStart, refreshedAt := time.Time{}, time.Time{}
	if periodStartMillis > 0 {
		periodStart = time.UnixMilli(periodStartMillis).UTC()
	}
	if refreshedAtMillis > 0 {
		refreshedAt = time.UnixMilli(refreshedAtMillis).UTC()
	}
	projection, err := domain.ValidateStatistic(metricKey, dimensionsJSON, value, periodStart, refreshedAt)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.GetStatistic(ctx, projection.MetricKey, projection.PeriodStart, projection.DimensionsHash)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if current != nil && !projection.RefreshedAt.After(current.RefreshedAt) {
		return current, nil
	}
	if err := s.repo.UpsertStatistic(ctx, projection); err != nil {
		return nil, err
	}
	return s.repo.GetStatistic(ctx, projection.MetricKey, projection.PeriodStart, projection.DimensionsHash)
}
func (s *StatisticsService) List(ctx context.Context, actor domain.Actor, metricKey string, fromMillis, toMillis int64) ([]domain.PlatformStatistic, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, err
	}
	filter, err := domain.ValidateStatisticFilter(metricKey, fromMillis, toMillis)
	if err != nil {
		return nil, err
	}
	return s.repo.ListStatistics(ctx, filter)
}
