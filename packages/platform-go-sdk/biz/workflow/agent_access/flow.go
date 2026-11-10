package agent_access

import (
	"context"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
)

type API interface {
	EnsureAgent(ctx context.Context, authorization string, name string, input contract.AgentConfiguration) (contract.Workspace, error)
	GetAgent(ctx context.Context, authorization string, name string) (contract.Workspace, error)
	CreateModelConnection(ctx context.Context, authorization string, input contract.ModelConnectionInput) (contract.ModelConnection, error)
	UpdateModelConnection(ctx context.Context, authorization string, id string, input contract.ModelConnectionUpdate) (contract.ModelConnection, error)
	RevokeModelConnection(ctx context.Context, authorization string, id string, version int64) (contract.ModelConnection, error)
	ListModelConnections(ctx context.Context, authorization string) ([]contract.ModelConnection, error)
}
type Flow struct {
	api        API
	token      func(context.Context) (string, error)
	invalidate func(error)
}

func New(api API, token func(context.Context) (string, error), invalidate func(error)) *Flow {
	return &Flow{api, token, invalidate}
}
func (u *Flow) EnsureAgent(ctx context.Context, name string, input contract.AgentConfiguration) (contract.Workspace, error) {
	var zero contract.Workspace
	if e := contract.ValidateSegments(name); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.EnsureAgent(ctx, "Bearer "+token, name, input)
	u.invalidate(e)
	return v, e
}
func (u *Flow) GetAgent(ctx context.Context, name string) (contract.Workspace, error) {
	var zero contract.Workspace
	if e := contract.ValidateSegments(name); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.GetAgent(ctx, "Bearer "+token, name)
	u.invalidate(e)
	return v, e
}
func (u *Flow) CreateModelConnection(ctx context.Context, input contract.ModelConnectionInput) (contract.ModelConnection, error) {
	var zero contract.ModelConnection
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.CreateModelConnection(ctx, "Bearer "+token, input)
	u.invalidate(e)
	return v, e
}
func (u *Flow) UpdateModelConnection(ctx context.Context, id string, input contract.ModelConnectionUpdate) (contract.ModelConnection, error) {
	var zero contract.ModelConnection
	if e := contract.ValidateSegments(id); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.UpdateModelConnection(ctx, "Bearer "+token, id, input)
	u.invalidate(e)
	return v, e
}
func (u *Flow) RevokeModelConnection(ctx context.Context, id string, version int64) (contract.ModelConnection, error) {
	var zero contract.ModelConnection
	if e := contract.ValidateSegments(id); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.RevokeModelConnection(ctx, "Bearer "+token, id, version)
	u.invalidate(e)
	return v, e
}
func (u *Flow) ListModelConnections(ctx context.Context) ([]contract.ModelConnection, error) {
	var zero []contract.ModelConnection
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.ListModelConnections(ctx, "Bearer "+token)
	u.invalidate(e)
	return v, e
}
