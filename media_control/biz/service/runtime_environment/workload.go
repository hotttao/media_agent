package runtime_environment

import (
	"context"
	"errors"
	d "media_agent/media_control/biz/domain/runtime_environment"
)

func (s *Service) Workload(ctx context.Context, ws string) (*d.WorkloadTarget, error) {
	return s.repo.(d.WorkloadRepository).Workload(ctx, ws, false)
}
func (s *Service) ListWorkloads(ctx context.Context, after string) ([]d.WorkloadTarget, error) {
	return s.repo.(d.WorkloadRepository).ListWorkloads(ctx, after, 100)
}
func (s *Service) SetWorkload(ctx context.Context, v d.WorkloadTarget, expected int64, key, hash string) (*d.WorkloadTarget, error) {
	if v.WorkspaceID == "" || v.AgentConfigID == "" || v.EnvironmentID == "" || v.OwnerUserID == "" || v.DesiredReplicas < 0 || v.DesiredReplicas > 64 || v.WorkspaceVersion < 1 || v.Replicas < 0 || v.Replicas > 64 || expected < 0 || key == "" || len(key) > 128 || hash == "" || len(v.Spec) == 0 {
		return nil, d.ErrConflict
	}
	repo := s.repo.(d.WorkloadRepository)
	var result *d.WorkloadTarget
	e := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		previous, e := repo.Workload(ctx, v.WorkspaceID, true)
		if e != nil && !errors.Is(e, d.ErrNotFound) {
			return e
		}
		request, e := repo.WorkloadRequest(ctx, v.WorkspaceID, key)
		if e == nil {
			if request.RequestHash != hash {
				return d.ErrConflict
			}
			result = &request.Target
			return nil
		}
		if !errors.Is(e, d.ErrNotFound) {
			return e
		}
		if previous != nil {
			if previous.Version != expected || previous.AgentConfigID != v.AgentConfigID || previous.EnvironmentID != v.EnvironmentID {
				return d.ErrConflict
			}
			v.Spec = previous.Spec
			v.OwnerUserID = previous.OwnerUserID
		} else if expected != 0 {
			return d.ErrConflict
		}
		v.Version = expected + 1
		if e = repo.SaveWorkload(ctx, v, expected, d.WorkloadRequest{WorkspaceID: v.WorkspaceID, IdempotencyKey: key, RequestHash: hash, Target: v}); e != nil {
			return e
		}
		result = &v
		return nil
	})
	return result, e
}
