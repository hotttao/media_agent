package platform_administration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/platform_administration"
)

type fakeClusterRepository struct {
	values map[string]*domain.ClusterProjection
}

func (r *fakeClusterRepository) UpsertCluster(_ context.Context, value *domain.ClusterProjection) error {
	current := r.values[value.ClusterID]
	if current != nil && !value.ObservedAt.After(current.ObservedAt) {
		return nil
	}
	copy := *value
	r.values[value.ClusterID] = &copy
	return nil
}
func (r *fakeClusterRepository) ListClusters(context.Context) ([]domain.ClusterProjection, error) {
	result := make([]domain.ClusterProjection, 0, len(r.values))
	for _, value := range r.values {
		result = append(result, *value)
	}
	return result, nil
}
func (r *fakeClusterRepository) GetCluster(_ context.Context, id string) (*domain.ClusterProjection, error) {
	value := r.values[id]
	if value == nil {
		return nil, domain.ErrNotFound
	}
	copy := *value
	return &copy, nil
}

func TestClusterProjectionRejectsLateObservation(t *testing.T) {
	repository := &fakeClusterRepository{values: map[string]*domain.ClusterProjection{}}
	service := NewClusterService(repository)
	system := domain.Actor{UserID: "observer-1", Role: "system", Internal: true}
	newer := time.Unix(200, 0).UnixMilli()
	value, err := service.Report(context.Background(), system, "cluster-a", "online", `{"cpu":8}`, newer)
	if err != nil || value.Status != domain.ClusterOnline {
		t.Fatalf("new report=%+v err=%v", value, err)
	}
	value, err = service.Report(context.Background(), system, "cluster-a", "offline", `{"cpu":0}`, newer-1000)
	if err != nil || value.Status != domain.ClusterOnline || value.CapacityJSON != `{"cpu":8}` {
		t.Fatalf("late report overwrote projection: %+v err=%v", value, err)
	}
	value, err = service.Report(context.Background(), system, "cluster-a", "degraded", `{"cpu":4}`, newer+1000)
	if err != nil || value.Status != domain.ClusterDegraded {
		t.Fatalf("new report not applied: %+v err=%v", value, err)
	}
}

func TestClusterProjectionRequiresInternalSystemReporter(t *testing.T) {
	service := NewClusterService(&fakeClusterRepository{values: map[string]*domain.ClusterProjection{}})
	for _, actor := range []domain.Actor{
		{UserID: "user", Role: "user"},
		{UserID: "admin", Role: "admin", Internal: true},
		{UserID: "system", Role: "system", Internal: false},
	} {
		if _, err := service.Report(context.Background(), actor, "cluster-a", "online", `{}`, 1); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("reporter accepted: %+v err=%v", actor, err)
		}
	}
}
