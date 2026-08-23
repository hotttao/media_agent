package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	chatpb "media_agent/hertz_gen/model/media_agent_gateway/chat"
	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
	"media_agent/media_agent_gateway/biz/middleware"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
	"media_agent/media_agent_gateway/biz/workflow"
)

var sessions *service.Service
var runTurn *workflow.RunTurn
var eventStream *workflow.EventStream

func Configure(sessionService *service.Service, turnFlow *workflow.RunTurn, source sessionevents.Source) {
	sessions = sessionService
	runTurn = turnFlow
	eventStream = workflow.NewEventStream(turnFlow, source)
}

func GetDefaultSession(ctx context.Context, c *app.RequestContext) {
	value, err := sessions.DefaultSession(ctx, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	state, err := runTurn.Register(ctx, value, string(c.Cookie("token")))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(consts.StatusOK, toSessionResponse(value, state.ChatStatus))
}

func CreateSession(ctx context.Context, c *app.RequestContext) {
	var req chatpb.CreateSessionRequest
	if err := c.BindAndValidate(&req); err != nil || req.WorkspaceId == "" {
		if err == nil {
			err = errors.New("workspace_id is required")
		}
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	value, err := sessions.Create(ctx, middleware.UserID(ctx), req.WorkspaceId, req.Title, string(c.Cookie("token")))
	if err != nil {
		writeError(c, err)
		return
	}
	state, err := runTurn.Register(ctx, value, string(c.Cookie("token")))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(consts.StatusCreated, toSessionResponse(value, state.ChatStatus))
}

func ListSessions(ctx context.Context, c *app.RequestContext) {
	var req chatpb.ListSessionsRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	values, err := sessions.List(ctx, middleware.UserID(ctx), req.WorkspaceId)
	if err != nil {
		writeError(c, err)
		return
	}
	resp := &chatpb.ListSessionsResponse{Sessions: make([]*chatpb.SessionResponse, 0, len(values))}
	for i := range values {
		chatStatus := domain.ChatStatusHuman
		if state, stateErr := runTurn.State(ctx, values[i].ID, middleware.UserID(ctx)); stateErr == nil {
			chatStatus = state.ChatStatus
		}
		resp.Sessions = append(resp.Sessions, toSessionResponse(&values[i], chatStatus))
	}
	c.JSON(consts.StatusOK, resp)
}

func ListMessages(ctx context.Context, c *app.RequestContext) {
	var req chatpb.ListMessagesRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	values, err := runTurn.Messages(ctx, req.SessionId, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	resp := &chatpb.ListMessagesResponse{Messages: make([]*chatpb.MessageResponse, 0, len(values))}
	for _, value := range values {
		createdAt, _ := time.Parse(time.RFC3339, value.CreatedAt)
		resp.Messages = append(resp.Messages, &chatpb.MessageResponse{Id: value.MessageID, TurnId: value.TurnID, Role: value.Role, ContentJson: messageContent(value.Role, value.ContentRef), Sequence: value.Sequence, CreatedAt: createdAt.UnixMilli()})
	}
	c.JSON(consts.StatusOK, resp)
}

func RunTurn(ctx context.Context, c *app.RequestContext) {
	var req chatpb.RunTurnRequest
	if err := c.BindAndValidate(&req); err != nil || req.Message == "" {
		if err == nil {
			err = errors.New("message is required")
		}
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	turn, err := runTurn.Start(ctx, req.SessionId, middleware.UserID(ctx), string(c.Cookie("token")), req.IdempotencyKey, req.Message)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, &chatpb.RunTurnResponse{TurnId: turn.TurnID, Status: turn.Status})
}

func InterruptTurn(ctx context.Context, c *app.RequestContext) {
	var req chatpb.InterruptTurnRequest
	if err := c.BindAndValidate(&req); err != nil || req.SessionId == "" {
		if err == nil {
			err = errors.New("session_id is required")
		}
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	turn, err := runTurn.Interrupt(ctx, req.SessionId, middleware.UserID(ctx), string(c.Cookie("token")))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(consts.StatusAccepted, &chatpb.InterruptTurnResponse{TurnId: turn.TurnID, Status: "interrupting"})
}

func toSessionResponse(value *domain.Session, chatStatus string) *chatpb.SessionResponse {
	return &chatpb.SessionResponse{Id: value.ID, WorkspaceId: value.WorkspaceID, AgentId: value.AgentID, AgentType: value.AgentType, Title: value.Title, Status: value.Status, ChatStatus: chatStatus, ContextVersion: value.ContextVersion, CreatedAt: value.CreatedAt.UnixMilli(), UpdatedAt: value.UpdatedAt.UnixMilli()}
}

func writeError(c *app.RequestContext, err error) {
	status := consts.StatusInternalServerError
	if errors.Is(err, domain.ErrNotFound) {
		status = consts.StatusNotFound
	}
	if errors.Is(err, domain.ErrForbidden) {
		status = consts.StatusForbidden
	}
	if errors.Is(err, domain.ErrTurnInFlight) || errors.Is(err, domain.ErrAlreadyExists) || errors.Is(err, domain.ErrContextChanged) {
		status = consts.StatusConflict
	}
	if errors.Is(err, domain.ErrInactive) {
		status = consts.StatusConflict
	}
	c.JSON(status, map[string]string{"error": err.Error()})
}

// SubscribeEvents .
// @router /api/chat/sessions/:session_id/events [GET]
func SubscribeEvents(ctx context.Context, c *app.RequestContext) {
	var req chatpb.SubscribeEventsRequest
	if err := c.BindAndValidate(&req); err != nil || req.AfterSequence < 0 {
		if err == nil {
			err = errors.New("after_sequence must not be negative")
		}
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if _, err := sessions.Authorize(ctx, req.SessionId, middleware.UserID(ctx)); err != nil {
		writeError(c, err)
		return
	}
	writer := sse.NewWriter(c)
	if err := writer.WriteEvent("", "ready", []byte(`{}`)); err != nil {
		return
	}
	_ = eventStream.Stream(ctx, req.SessionId, middleware.UserID(ctx), req.AfterSequence, func(event workflow.StreamEvent) error {
		if event.Heartbeat {
			return writer.WriteEvent("", "heartbeat", []byte(`{}`))
		}
		id := ""
		if event.Sequence > 0 {
			id = strconv.FormatInt(event.Sequence, 10)
		}
		return writer.WriteEvent(id, "message", event.Payload)
	})
}

func messageContent(role string, content json.RawMessage) string {
	if role != "user" {
		return string(content)
	}
	var input struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(content, &input) != nil || input.Message == "" {
		return string(content)
	}
	encoded, _ := json.Marshal([]map[string]string{{"type": "text", "text": input.Message}})
	return string(encoded)
}

// ListAgentRequests .
// @router /api/chat/sessions/:session_id/requests [GET]
func ListAgentRequests(ctx context.Context, c *app.RequestContext) {
	var req chatpb.ListAgentRequestsRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	values, err := runTurn.AgentRequests(ctx, req.SessionId, middleware.UserID(ctx), req.TurnId)
	if err != nil {
		writeError(c, err)
		return
	}
	resp := &chatpb.ListAgentRequestsResponse{Requests: make([]*chatpb.AgentRequestResponse, 0, len(values))}
	for i := range values {
		resp.Requests = append(resp.Requests, toAgentRequestResponse(&values[i]))
	}
	c.JSON(consts.StatusOK, resp)
}

// ResolveAgentRequest .
// @router /api/chat/sessions/:session_id/turns/:turn_id/requests/:request_id/resolve [POST]
func ResolveAgentRequest(ctx context.Context, c *app.RequestContext) {
	var req chatpb.ResolveAgentRequestRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var response map[string]any
	if req.ResponseJson == "" || json.Unmarshal([]byte(req.ResponseJson), &response) != nil || response == nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": "response_json must be a JSON object"})
		return
	}
	value, err := runTurn.ResolveAgentRequest(ctx, req.SessionId, middleware.UserID(ctx), req.TurnId, req.RequestId, response)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(consts.StatusOK, toAgentRequestResponse(value))
}

func toAgentRequestResponse(value *client.AgentRequest) *chatpb.AgentRequestResponse {
	createdAt, _ := time.Parse(time.RFC3339, value.CreatedAt)
	resolvedAt, _ := time.Parse(time.RFC3339, value.ResolvedAt)
	return &chatpb.AgentRequestResponse{RequestId: value.RequestID, TurnId: value.TurnID, RequestType: value.RequestType, PayloadJson: string(value.Payload), Status: value.Status, ResponseJson: string(value.Response), CreatedAt: createdAt.UnixMilli(), ResolvedAt: resolvedAt.UnixMilli()}
}
