package workspace

import (
	"context"
	d "media_agent/media_control/biz/domain/workspace"
)

type ApplicationBindings struct{ repo d.ApplicationBindings }

func NewApplicationBindings(repo d.ApplicationBindings) *ApplicationBindings {
	return &ApplicationBindings{repo}
}
func (s *ApplicationBindings) Lock(ctx context.Context, b d.ApplicationBinding) error {
	return s.repo.LockApplicationKey(ctx, b)
}
func (s *ApplicationBindings) Get(ctx context.Context, b d.ApplicationBinding) (d.ApplicationBinding, error) {
	return s.repo.FindApplicationBinding(ctx, b)
}
func (s *ApplicationBindings) ByWorkspace(ctx context.Context, b d.ApplicationBinding) (d.ApplicationBinding, error) {
	return s.repo.FindApplicationWorkspace(ctx, b)
}
func (s *ApplicationBindings) Create(ctx context.Context, b d.ApplicationBinding) error {
	return s.repo.CreateApplicationBinding(ctx, b)
}
