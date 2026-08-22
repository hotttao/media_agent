package platform_administration

import (
	"context"
	"strings"
	"time"

	domain "media_agent/media_control/biz/domain/platform_administration"
)

type ClusterService struct{ repo domain.ClusterRepository }

func NewClusterService(repo domain.ClusterRepository) *ClusterService {
	return &ClusterService{repo: repo}
}

func (s *ClusterService) Report(ctx context.Context, actor domain.Actor, clusterID, status, capacityJSON string, observedAtMillis int64) (*domain.ClusterProjection, error) {
	if err := actor.ValidateSystem(); err != nil {
		return nil, err
	}
	observedAt := time.Time{}
	if observedAtMillis > 0 {
		observedAt = time.UnixMilli(observedAtMillis).UTC()
	}
	clusterID, parsedStatus, capacityJSON, err := domain.ValidateCluster(clusterID, status, capacityJSON, observedAt)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.GetCluster(ctx, clusterID)
	if err != nil && err != domain.ErrNotFound {
		return nil, err
	}
	if current != nil && !observedAt.After(current.ObservedAt) {
		return current, nil
	}
	value := &domain.ClusterProjection{ClusterID: clusterID, Status: parsedStatus, CapacityJSON: capacityJSON, ObservedAt: observedAt}
	if err := s.repo.UpsertCluster(ctx, value); err != nil {
		return nil, err
	}
	return s.repo.GetCluster(ctx, clusterID)
}
func (s *ClusterService) List(ctx context.Context, actor domain.Actor) ([]domain.ClusterProjection, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, err
	}
	return s.repo.ListClusters(ctx)
}
func (s *ClusterService) Get(ctx context.Context, actor domain.Actor, clusterID string) (*domain.ClusterProjection, error) {
	if err := actor.ValidateAdmin(); err != nil {
		return nil, err
	}
	return s.repo.GetCluster(ctx, strings.TrimSpace(clusterID))
}
