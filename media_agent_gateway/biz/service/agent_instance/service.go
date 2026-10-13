package agent_instance

import (
	"context"
	d "media_agent/media_agent_gateway/biz/domain/agent_instance"
	"time"
)

type TxRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}
type Service struct {
	repo d.Repository
	tx   TxRunner
}

func New(repo d.Repository, tx TxRunner) *Service { return &Service{repo, tx} }
func (s *Service) Get(ctx context.Context, env, ws, cfg, replica string) (*d.Instance, error) {
	return s.repo.Get(ctx, env, ws, cfg, replica, false)
}
func (s *Service) Historical(ctx context.Context, env, ws, cfg, replica, inc string) (*d.Instance, error) {
	return s.repo.Historical(ctx, env, ws, cfg, replica, inc)
}
func (s *Service) List(ctx context.Context, env, ws, cfg string) ([]d.Instance, error) {
	return s.repo.List(ctx, env, ws, cfg)
}
func (s *Service) Register(ctx context.Context, candidate d.Instance, expected int64) (*d.Instance, error) {
	if candidate.EnvironmentID == "" || candidate.WorkspaceID == "" || candidate.AgentConfigID == "" || candidate.ReplicaID == "" || candidate.IncarnationID == "" || candidate.ResourceUID == "" || candidate.ResourceGeneration == "" || candidate.Endpoint == "" || candidate.CredentialHash == "" || candidate.CredentialKeyID == "" || expected < 0 {
		return nil, d.ErrInvalid
	}
	var result *d.Instance
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		existing, err := s.repo.Get(ctx, candidate.EnvironmentID, candidate.WorkspaceID, candidate.AgentConfigID, candidate.ReplicaID, true)
		if err != nil && err != d.ErrNotFound {
			return err
		}
		if existing != nil {
			if existing.IncarnationID == candidate.IncarnationID && (existing.ResourceUID != candidate.ResourceUID || existing.ResourceGeneration != candidate.ResourceGeneration) {
				return d.ErrIncarnationConflict
			}
			if existing.ResourceUID == candidate.ResourceUID && existing.ResourceGeneration == candidate.ResourceGeneration {
				if existing.IncarnationID != candidate.IncarnationID {
					return d.ErrIncarnationConflict
				}
				result = existing
				return nil // Retain admission, key version and credential on every retry.
			}
			if existing.Version != expected {
				return d.ErrConflict
			}
			if existing.Admission == "retired" {
				candidate.Admission = "active"
				candidate.DrainOperationID = ""
			} else {
				candidate.Admission = existing.Admission
				candidate.DrainOperationID = existing.DrainOperationID
			}
		} else if expected != 0 {
			return d.ErrConflict
		}
		candidate.Version = expected + 1
		candidate.UpdatedAt = time.Now().UTC()
		if err := s.repo.Save(ctx, &candidate, expected); err != nil {
			return err
		}
		result = &candidate
		return nil
	})
	return result, err
}
func (s *Service) Drain(ctx context.Context, instance *d.Instance, operation string) error {
	if operation == "" || len(operation) > 128 {
		return d.ErrInvalid
	}
	return s.tx.RunInTx(ctx, func(ctx context.Context) error {
		current, err := s.repo.Get(ctx, instance.EnvironmentID, instance.WorkspaceID, instance.AgentConfigID, instance.ReplicaID, true)
		if err != nil {
			return err
		}
		if current.IncarnationID != instance.IncarnationID {
			return d.ErrConflict
		}
		if current.Admission == "draining" {
			if current.DrainOperationID == operation {
				return nil
			}
			return d.ErrConflict
		}
		return s.repo.Drain(ctx, current, operation)
	})
}

func (s *Service) Retire(ctx context.Context, v *d.Instance, operation string) error {
	return s.tx.RunInTx(ctx, func(ctx context.Context) error {
		current, e := s.repo.Get(ctx, v.EnvironmentID, v.WorkspaceID, v.AgentConfigID, v.ReplicaID, true)
		if e != nil {
			return e
		}
		if current.IncarnationID != v.IncarnationID {
			return d.ErrConflict
		}
		if current.Admission == "retired" {
			return nil
		}
		if current.Admission != "draining" || current.DrainOperationID != operation {
			return d.ErrConflict
		}
		retirer, ok := s.repo.(interface {
			Retire(context.Context, *d.Instance, string) error
		})
		if !ok {
			return d.ErrInvalid
		}
		return retirer.Retire(ctx, current, operation)
	})
}
