package platform_administration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/platform_administration"
)

type fakeStatisticsRepository struct {
	values map[string]*domain.PlatformStatistic
}

func statisticID(metric string, period time.Time, dimensionsHash string) string {
	return metric + period.String() + dimensionsHash
}
func (r *fakeStatisticsRepository) UpsertStatistic(_ context.Context, value *domain.PlatformStatistic) error {
	id := statisticID(value.MetricKey, value.PeriodStart, value.DimensionsHash)
	current := r.values[id]
	if current != nil && !value.RefreshedAt.After(current.RefreshedAt) {
		return nil
	}
	copy := *value
	r.values[id] = &copy
	return nil
}
func (r *fakeStatisticsRepository) GetStatistic(_ context.Context, metric string, period time.Time, dimensionsHash string) (*domain.PlatformStatistic, error) {
	value := r.values[statisticID(metric, period, dimensionsHash)]
	if value == nil {
		return nil, domain.ErrNotFound
	}
	copy := *value
	return &copy, nil
}
func (r *fakeStatisticsRepository) ListStatistics(_ context.Context, filter domain.StatisticFilter) ([]domain.PlatformStatistic, error) {
	result := make([]domain.PlatformStatistic, 0, len(r.values))
	for _, value := range r.values {
		if filter.MetricKey == "" || value.MetricKey == filter.MetricKey {
			result = append(result, *value)
		}
	}
	return result, nil
}

func TestPlatformStatisticIdempotencyAndLateRefresh(t *testing.T) {
	repository := &fakeStatisticsRepository{values: map[string]*domain.PlatformStatistic{}}
	service := NewStatisticsService(repository)
	system := domain.Actor{UserID: "observer", Role: "system", Internal: true}
	period, refreshed := time.Unix(100, 0).UnixMilli(), time.Unix(200, 0).UnixMilli()
	value, err := service.Report(context.Background(), system, "workspace.active", period, `{"region":"cn","tier":"gpu"}`, "10.50", refreshed)
	if err != nil || value.Value != "10.50" {
		t.Fatalf("first=%+v err=%v", value, err)
	}
	value, err = service.Report(context.Background(), system, "workspace.active", period, `{"tier":"gpu","region":"cn"}`, "1", refreshed-1000)
	if err != nil || value.Value != "10.50" || len(repository.values) != 1 {
		t.Fatalf("late/reordered refresh changed projection: %+v count=%d err=%v", value, len(repository.values), err)
	}
	value, err = service.Report(context.Background(), system, "workspace.active", period, `{"region":"cn","tier":"gpu"}`, "12", refreshed+1000)
	if err != nil || value.Value != "12" {
		t.Fatalf("new refresh=%+v err=%v", value, err)
	}
}

func TestPlatformStatisticRequiresInternalSystem(t *testing.T) {
	service := NewStatisticsService(&fakeStatisticsRepository{values: map[string]*domain.PlatformStatistic{}})
	if _, err := service.Report(context.Background(), domain.Actor{UserID: "admin", Role: "admin", Internal: true}, "workspace.active", 1, `{}`, "1", 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin reporter accepted: %v", err)
	}
}
