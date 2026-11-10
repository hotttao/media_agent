package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/protocol"
	chatpb "media_agent/hertz_gen/model/media_agent_gateway/chat"
	authpb "media_agent/hertz_gen/model/media_auth/application"
	connectionpb "media_agent/hertz_gen/model/media_control/model_connection"
	controlpb "media_agent/hertz_gen/model/media_control/workspace"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	"time"
)

func result(ctx context.Context, raw *protocol.Response, err error, source, out any) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var platformError *contract.Error
	if errors.As(err, &platformError) {
		return platformError
	}
	if raw != nil && (raw.StatusCode() < 200 || raw.StatusCode() >= 300) {
		return &contract.Error{StatusCode: raw.StatusCode()}
	}
	if err != nil || raw == nil {
		return fmt.Errorf("platform transport failed (%T)", err)
	}
	b, e := json.Marshal(source)
	if e != nil {
		return errors.New("invalid platform response")
	}
	if json.Unmarshal(b, out) != nil {
		return errors.New("invalid platform response")
	}
	return nil
}
func (c *Clients) UpsertUser(ctx context.Context, authorization string, externalID string, profile contract.UserProfileInput) (contract.UserProfile, error) {
	var out contract.UserProfile
	value, raw, e := c.auth.UpsertApplicationUser(withAuthorization(ctx, authorization), &authpb.UpsertApplicationUserRequest{ExternalId: externalID, DisplayName: profile.DisplayName, Email: profile.Email}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) GetUser(ctx context.Context, authorization string, externalID string) (contract.UserProfile, error) {
	var out contract.UserProfile
	value, raw, e := c.auth.GetApplicationUser(withAuthorization(ctx, authorization), &authpb.ApplicationUserRequest{ExternalId: externalID}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) Me(ctx context.Context, authorization string) (contract.UserProfile, error) {
	var out contract.UserProfile
	value, raw, e := c.auth.GetApplicationUserInfo(withAuthorization(ctx, authorization), &authpb.Empty{}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) EnsureAgent(ctx context.Context, authorization string, name string, input contract.AgentConfiguration) (contract.Workspace, error) {
	var out contract.Workspace
	value, raw, e := c.control.EnsureApplicationAgent(withAuthorization(ctx, authorization), &controlpb.EnsureApplicationAgentRequest{Name: name, DisplayName: input.DisplayName, AgentType: input.AgentType, ApiKey: input.APIKey, ModelConnectionId: input.ModelConnectionID, ModelName: input.ModelName, ModelProtocol: input.ModelProtocol, ModelBaseUrl: input.ModelBaseURL}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) GetAgent(ctx context.Context, authorization string, name string) (contract.Workspace, error) {
	var out contract.Workspace
	value, raw, e := c.control.GetApplicationAgent(withAuthorization(ctx, authorization), &controlpb.GetApplicationAgentRequest{Name: name}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) CreateModelConnection(ctx context.Context, authorization string, input contract.ModelConnectionInput) (contract.ModelConnection, error) {
	var out contract.ModelConnection
	value, raw, e := c.connections.CreateApplicationConnection(withAuthorization(ctx, authorization), &connectionpb.Command{Name: input.Name, AuthJson: input.AuthJSON}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) UpdateModelConnection(ctx context.Context, authorization string, id string, input contract.ModelConnectionUpdate) (contract.ModelConnection, error) {
	var out contract.ModelConnection
	value, raw, e := c.connections.UpdateApplicationConnection(withAuthorization(ctx, authorization), &connectionpb.Command{ConnectionId: id, Name: input.Name, AuthJson: input.AuthJSON, ExpectedVersion: input.ExpectedVersion}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) RevokeModelConnection(ctx context.Context, authorization string, id string, version int64) (contract.ModelConnection, error) {
	var out contract.ModelConnection
	value, raw, e := c.connections.RevokeApplicationConnection(withAuthorization(ctx, authorization), &connectionpb.Command{ConnectionId: id, ExpectedVersion: version}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) CreateSession(ctx context.Context, authorization string, workspaceID, title string) (contract.Session, error) {
	var out contract.Session
	value, raw, e := c.gateway.ApplicationCreateSession(withAuthorization(ctx, authorization), &chatpb.CreateSessionRequest{WorkspaceId: workspaceID, Title: title}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) RunTurn(ctx context.Context, authorization string, id, message, key string) (contract.TurnResult, error) {
	var out contract.TurnResult
	value, raw, e := c.gateway.ApplicationRunTurn(withAuthorization(ctx, authorization), &chatpb.RunTurnRequest{SessionId: id, Message: message, IdempotencyKey: key}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) Interrupt(ctx context.Context, authorization string, id string) (contract.TurnResult, error) {
	var out contract.TurnResult
	value, raw, e := c.gateway.ApplicationInterruptTurn(withAuthorization(ctx, authorization), &chatpb.InterruptTurnRequest{SessionId: id}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) ResolveRequest(ctx context.Context, authorization string, id, turnID, requestID, responseJSON string) (contract.AgentRequest, error) {
	var out contract.AgentRequest
	value, raw, e := c.gateway.ApplicationResolveAgentRequest(withAuthorization(ctx, authorization), &chatpb.ResolveAgentRequestRequest{SessionId: id, TurnId: turnID, RequestId: requestID, ResponseJson: responseJSON}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value, &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) ListModelConnections(ctx context.Context, authorization string) ([]contract.ModelConnection, error) {
	var out []contract.ModelConnection
	value, raw, e := c.connections.ListApplicationConnections(withAuthorization(ctx, authorization), &connectionpb.Query{}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value.GetConnections(), &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) ListSessions(ctx context.Context, authorization string, id string) ([]contract.Session, error) {
	var out []contract.Session
	value, raw, e := c.gateway.ApplicationListSessions(withAuthorization(ctx, authorization), &chatpb.ListSessionsRequest{WorkspaceId: id}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value.GetSessions(), &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) ListMessages(ctx context.Context, authorization string, id string) ([]contract.Message, error) {
	var out []contract.Message
	value, raw, e := c.gateway.ApplicationListMessages(withAuthorization(ctx, authorization), &chatpb.ListMessagesRequest{SessionId: id}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value.GetMessages(), &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) ListRequests(ctx context.Context, authorization string, id string) ([]contract.AgentRequest, error) {
	var out []contract.AgentRequest
	value, raw, e := c.gateway.ApplicationListAgentRequests(withAuthorization(ctx, authorization), &chatpb.ListAgentRequestsRequest{SessionId: id}, config.WithRequestTimeout(30*time.Second))
	if e == nil && value != nil {
		e = result(ctx, raw, e, value.GetRequests(), &out)
	} else {
		e = result(ctx, raw, e, nil, &out)
	}
	return out, e
}
func (c *Clients) Exchange(ctx context.Context, authorization, externalID, teamID string, scopes []string) (contract.Token, error) {
	var out contract.Token
	v, r, e := c.auth.IssueApplicationUserToken(withAuthorization(ctx, authorization), &authpb.IssueApplicationTokenRequest{ExternalId: externalID, TeamId: teamID, Scopes: scopes}, config.WithRequestTimeout(30*time.Second))
	e = result(ctx, r, e, v, &out)
	return out, e
}
