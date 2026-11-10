package session_relay

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	"net/http"
	"strings"
)

type API interface {
	Events(context.Context, string, string, int64) (*http.Response, error)
	CreateSession(ctx context.Context, authorization string, workspaceID, title string) (contract.Session, error)
	RunTurn(ctx context.Context, authorization string, id, message, key string) (contract.TurnResult, error)
	Interrupt(ctx context.Context, authorization string, id string) (contract.TurnResult, error)
	ResolveRequest(ctx context.Context, authorization string, id, turnID, requestID, responseJSON string) (contract.AgentRequest, error)
	ListSessions(ctx context.Context, authorization string, id string) ([]contract.Session, error)
	ListMessages(ctx context.Context, authorization string, id string) ([]contract.Message, error)
	ListRequests(ctx context.Context, authorization string, id string) ([]contract.AgentRequest, error)
}
type Flow struct {
	api        API
	token      func(context.Context) (string, error)
	invalidate func(error)
}

func New(api API, token func(context.Context) (string, error), invalidate func(error)) *Flow {
	return &Flow{api, token, invalidate}
}
func (u *Flow) CreateSession(ctx context.Context, workspaceID, title string) (contract.Session, error) {
	var zero contract.Session
	if e := contract.ValidateSegments(workspaceID); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.CreateSession(ctx, "Bearer "+token, workspaceID, title)
	u.invalidate(e)
	return v, e
}
func (u *Flow) RunTurn(ctx context.Context, id, message, key string) (contract.TurnResult, error) {
	var zero contract.TurnResult
	if e := contract.ValidateSegments(id); e != nil {
		return zero, e
	}
	if strings.TrimSpace(message) == "" || strings.TrimSpace(key) == "" {
		return zero, errors.New("message and idempotency key required")
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.RunTurn(ctx, "Bearer "+token, id, message, key)
	u.invalidate(e)
	return v, e
}
func (u *Flow) Interrupt(ctx context.Context, id string) (contract.TurnResult, error) {
	var zero contract.TurnResult
	if e := contract.ValidateSegments(id); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.Interrupt(ctx, "Bearer "+token, id)
	u.invalidate(e)
	return v, e
}
func (u *Flow) ResolveRequest(ctx context.Context, id, turnID, requestID, responseJSON string) (contract.AgentRequest, error) {
	var zero contract.AgentRequest
	if e := contract.ValidateSegments(id, turnID, requestID); e != nil {
		return zero, e
	}
	if !json.Valid([]byte(responseJSON)) {
		return zero, errors.New("invalid response JSON")
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.ResolveRequest(ctx, "Bearer "+token, id, turnID, requestID, responseJSON)
	u.invalidate(e)
	return v, e
}
func (u *Flow) ListSessions(ctx context.Context, id string) ([]contract.Session, error) {
	var zero []contract.Session
	if e := contract.ValidateSegments(id); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.ListSessions(ctx, "Bearer "+token, id)
	u.invalidate(e)
	return v, e
}
func (u *Flow) ListMessages(ctx context.Context, id string) ([]contract.Message, error) {
	var zero []contract.Message
	if e := contract.ValidateSegments(id); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.ListMessages(ctx, "Bearer "+token, id)
	u.invalidate(e)
	return v, e
}
func (u *Flow) ListRequests(ctx context.Context, id string) ([]contract.AgentRequest, error) {
	var zero []contract.AgentRequest
	if e := contract.ValidateSegments(id); e != nil {
		return zero, e
	}
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.ListRequests(ctx, "Bearer "+token, id)
	u.invalidate(e)
	return v, e
}

func (u *Flow) Events(ctx context.Context, id string, after int64) (*http.Response, error) {
	token, e := u.token(ctx)
	if e != nil {
		return nil, e
	}
	v, e := u.api.Events(ctx, "Bearer "+token, id, after)
	u.invalidate(e)
	return v, e
}
