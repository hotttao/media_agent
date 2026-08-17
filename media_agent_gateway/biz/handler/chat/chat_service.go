package chat

import (
	"context"
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
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
	"media_agent/media_agent_gateway/biz/workflow"
)

var sessions *service.Service
var runTurn *workflow.RunTurn
var eventHub *sessionevents.Broadcaster

func Configure(sessionService *service.Service, turnFlow *workflow.RunTurn, hub *sessionevents.Broadcaster) {
	sessions = sessionService
	runTurn = turnFlow
	eventHub = hub
}

func GetDefaultSession(ctx context.Context, c *app.RequestContext) {
	value, err := sessions.DefaultSession(ctx, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(consts.StatusOK, toSessionResponse(value))
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
	c.JSON(consts.StatusCreated, toSessionResponse(value))
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
		resp.Sessions = append(resp.Sessions, toSessionResponse(&values[i]))
	}
	c.JSON(consts.StatusOK, resp)
}

func ListMessages(ctx context.Context, c *app.RequestContext) {
	var req chatpb.ListMessagesRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	values, err := sessions.Messages(ctx, req.SessionId, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	resp := &chatpb.ListMessagesResponse{Messages: make([]*chatpb.MessageResponse, 0, len(values))}
	for _, value := range values {
		resp.Messages = append(resp.Messages, &chatpb.MessageResponse{Id: value.ID, TurnId: value.TurnID, Role: value.Role, ContentJson: string(value.Content), Sequence: value.Sequence, CreatedAt: value.CreatedAt.UnixMilli()})
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
	c.JSON(consts.StatusAccepted, &chatpb.RunTurnResponse{TurnId: turn.ID, Status: "accepted"})
}

func toSessionResponse(value *domain.Session) *chatpb.SessionResponse {
	return &chatpb.SessionResponse{Id: value.ID, WorkspaceId: value.WorkspaceID, AgentId: value.AgentID, AgentType: value.AgentType, Title: value.Title, Status: value.Status, ChatStatus: value.ChatStatus, CreatedAt: value.CreatedAt.UnixMilli(), UpdatedAt: value.UpdatedAt.UnixMilli()}
}

func writeError(c *app.RequestContext, err error) {
	status := consts.StatusInternalServerError
	if errors.Is(err, domain.ErrNotFound) {
		status = consts.StatusNotFound
	}
	if errors.Is(err, domain.ErrForbidden) {
		status = consts.StatusForbidden
	}
	if errors.Is(err, domain.ErrTurnInFlight) || errors.Is(err, domain.ErrAlreadyExists) {
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
	subscription := eventHub.Subscribe(req.SessionId)
	defer subscription.Close()
	replay, err := sessions.EventsAfter(ctx, req.SessionId, middleware.UserID(ctx), req.AfterSequence)
	if err != nil {
		writeError(c, err)
		return
	}
	writer := sse.NewWriter(c)
	if err := writer.WriteEvent("", "ready", []byte(`{}`)); err != nil {
		return
	}
	cursor := req.AfterSequence
	for _, event := range replay {
		if err := writer.WriteEvent(strconv.FormatInt(event.Sequence, 10), "message", event.Payload); err != nil {
			return
		}
		cursor = event.Sequence
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case event, ok := <-subscription.Events:
			if !ok {
				return
			}
			if event.Sequence == 0 {
				if err := writer.WriteEvent("", "message", event.Payload); err != nil {
					return
				}
				continue
			}
			if event.Sequence <= cursor {
				continue
			}
			if err := writer.WriteEvent(strconv.FormatInt(event.Sequence, 10), "message", event.Payload); err != nil {
				return
			}
			cursor = event.Sequence
		case <-heartbeat.C:
			if err := writer.WriteEvent("", "heartbeat", []byte(`{}`)); err != nil {
				return
			}
		}
	}
}
