package workflow

import (
	"context"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/policy/resource"
	"media_agent/media_workflow/biz/shared"
)

func (s *Service) lockResource(ctx context.Context, id string) (*domain.Workflow, error) {
	if repo, ok := s.repo.(interface {
		GetForUpdate(context.Context, string) (*domain.Workflow, error)
	}); ok {
		return repo.GetForUpdate(ctx, id)
	}
	if s.authority != nil {
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) writeResource(ctx context.Context, id, caller string, minimum resource.Level, teams []string, write func(context.Context, *domain.Workflow) error) error {
	if s.authority != nil {
		var err error
		ctx, _, err = shared.PrepareResourceFacts(ctx, caller, s.authority)
		if err != nil {
			return err
		}
	}
	run := func(txCtx context.Context) error {
		w, err := s.lockResource(txCtx, id)
		if err != nil {
			return err
		}
		level, err := s.permission(txCtx, w, caller, teams)
		if err != nil {
			return err
		}
		if level < minimum {
			return domain.ErrWorkflowForbidden
		}
		return write(txCtx, w)
	}
	if s.tx == nil {
		if s.authority != nil {
			return shared.ErrResourceAuthorityUnavailable
		}
		return run(ctx)
	}
	return s.tx.RunInTx(ctx, run)
}
