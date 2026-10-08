package template

import (
	"context"
	domain "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/policy/resource"
	"media_agent/media_workflow/biz/shared"
)

func (s *Service) lockResource(ctx context.Context, id string) (*domain.Template, error) {
	if repo, ok := s.repo.(interface {
		GetForUpdate(context.Context, string) (*domain.Template, error)
	}); ok {
		return repo.GetForUpdate(ctx, id)
	}
	if s.authority != nil {
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) writeResource(ctx context.Context, id, caller string, minimum resource.Level, teams []string, write func(context.Context, *domain.Template) error) error {
	if s.authority != nil {
		var err error
		ctx, _, err = shared.PrepareResourceFacts(ctx, caller, s.authority)
		if err != nil {
			return err
		}
	}
	run := func(txCtx context.Context) error {
		t, err := s.lockResource(txCtx, id)
		if err != nil {
			return err
		}
		level, err := s.permission(txCtx, t, caller, teams)
		if err != nil {
			return err
		}
		if level < minimum {
			return domain.ErrTemplateForbidden
		}
		return write(txCtx, t)
	}
	if s.tx == nil {
		if s.authority != nil {
			return shared.ErrResourceAuthorityUnavailable
		}
		return run(ctx)
	}
	return s.tx.RunInTx(ctx, run)
}
