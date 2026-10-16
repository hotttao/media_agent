package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
)

var ErrSessionPaused = errors.New("Session is paused. Resume it before sending another message.")

// RunTurn authorizes Gateway registrations and delegates all execution facts to Agent.
type RunTurn struct {
	sessions *service.Service
	runtimes client.Registry
	routing  client.RuntimeRouter
}

func NewRunTurn(sessions *service.Service, runtimes client.Registry) *RunTurn {
	return &RunTurn{sessions: sessions, runtimes: runtimes}
}

func (w *RunTurn) WithRouting(routing client.RuntimeRouter) *RunTurn { w.routing = routing; return w }

func (w *RunTurn) Register(ctx context.Context, session *domain.Session, authToken string) (*client.RuntimeSession, error) {
	if session.Status != domain.StatusStarting && session.Status != domain.StatusActive {
		return nil, domain.ErrInactive
	}
	runtime, err := w.registrationRuntime(ctx, session, authToken)
	if err != nil {
		return nil, err
	}
	contextRef := fmt.Sprintf("workspace:%s/agent:%s/context:%d", session.WorkspaceID, session.AgentID, session.ContextVersion)
	if session.MetadataTargetContextVersion != "" {
		contextRef = "metadata:" + session.AgentConfigID + ":" + session.MetadataTargetContextVersion
	}
	state, err := runtime.RegisterRuntimeSession(ctx, client.RegisterRuntimeSessionInput{
		SessionID:   session.ID,
		WorkspaceID: session.WorkspaceID, AgentConfigID: session.AgentConfigID,
		ContextVersion: session.ContextVersion,
		ContextRef:     contextRef,
	})
	if err != nil {
		// A transport or server failure is outcome-unknown: preserve starting so the
		// same session_id can be reconciled. A definitive client error is failed.
		if w.routing == nil && client.IsDefinitiveRegistrationFailure(err) && session.Status == domain.StatusStarting {
			_ = w.sessions.TransitionStatus(ctx, session, domain.StatusFailed)
		}
		return nil, err
	}
	if state.SessionID != session.ID || state.ContextVersion != session.ContextVersion {
		if session.Status == domain.StatusStarting {
			_ = w.sessions.TransitionStatus(ctx, session, domain.StatusFailed)
		}
		return nil, fmt.Errorf("Agent returned an incompatible Runtime Session registration")
	}
	if session.Status == domain.StatusStarting {
		if err := w.sessions.TransitionStatus(ctx, session, domain.StatusActive); err != nil {
			return nil, err
		}
	}
	return state, nil
}

// A new Docker instance normally needs a few seconds to register and become
// Ready. Reconcile the same Session while waiting, rather than failing creation.
func (w *RunTurn) registrationRuntime(ctx context.Context, session *domain.Session, token string) (client.Runtime, error) {
	waiting, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for {
		_, runtime, err := w.runtime(waiting, session, token)
		if !errors.Is(err, ErrRuntimeWaiting) {
			return runtime, err
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-waiting.Done():
			timer.Stop()
			return nil, ErrRuntimeWaiting
		case <-timer.C:
		}
	}
}

func (w *RunTurn) Start(ctx context.Context, sessionID, userID, authToken, key, message string) (*client.Turn, error) {
	session, err := w.sessions.Authorize(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	runtimeContext, runtime, err := w.runtime(ctx, session, authToken)
	if err != nil {
		return nil, err
	}
	state, err := w.Register(ctx, session, authToken)
	if err != nil {
		return nil, err
	}
	if state.Status == "paused" {
		return nil, ErrSessionPaused
	}
	if session.Status != domain.StatusActive {
		return nil, domain.ErrInactive
	}
	if strings.TrimSpace(key) == "" {
		key = uuid.NewString()
	}
	sessionToken, err := w.sessions.EnsureToken(ctx, session, true)
	if err != nil {
		return nil, err
	}
	apiKey := runtimeContext.APIKey
	if w.routing != nil {
		apiKey = ""
		sessionToken = ""
	} // Managed execution re-fetches credentials per claim.
	return runtime.SubmitTurn(ctx, client.SubmitTurnInput{
		SessionID:      session.ID,
		IdempotencyKey: key,
		ContextVersion: session.ContextVersion,
		Input:          map[string]any{"message": message},
		APIKey:         apiKey,
		SessionToken:   sessionToken,
	})
}

func (w *RunTurn) Interrupt(ctx context.Context, sessionID, userID, authToken string) (*client.Turn, error) {
	session, err := w.sessions.Authorize(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if session.Status != domain.StatusActive {
		return nil, domain.ErrInactive
	}
	_, runtime, err := w.ownerRuntime(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	state, err := runtime.GetRuntimeSession(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if state.ActiveTurnID == "" {
		return nil, domain.ErrNotFound
	}
	interrupted, err := runtime.InterruptTurn(ctx, session.ID, state.ActiveTurnID)
	if err != nil {
		return nil, err
	}
	if !interrupted {
		return nil, domain.ErrNotFound
	}
	return &client.Turn{TurnID: state.ActiveTurnID, SessionID: session.ID, Status: "interrupting"}, nil
}

func (w *RunTurn) Messages(ctx context.Context, sessionID, userID string) ([]client.Message, error) {
	session, runtime, err := w.authorizedRuntime(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	return runtime.ListMessages(ctx, session.ID, 1000)
}

func (w *RunTurn) AgentRequests(ctx context.Context, sessionID, userID, turnID string) ([]client.AgentRequest, error) {
	session, runtime, err := w.authorizedRuntime(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	return runtime.ListAgentRequests(ctx, session.ID, turnID)
}

func (w *RunTurn) ResolveAgentRequest(ctx context.Context, sessionID, userID, turnID, requestID string, response map[string]any) (*client.AgentRequest, error) {
	session, runtime, err := w.ownerRuntime(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	return runtime.ResolveAgentRequest(ctx, session.ID, turnID, requestID, response)
}

func (w *RunTurn) EventsAfter(ctx context.Context, sessionID, userID string, afterSequence int64) ([]client.UniversalEvent, error) {
	session, runtime, err := w.authorizedRuntime(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	const pageSize = 1000
	result := make([]client.UniversalEvent, 0, pageSize)
	cursor := afterSequence
	for {
		page, replayErr := runtime.ReplayEvents(ctx, session.ID, cursor, pageSize)
		if replayErr != nil {
			return nil, replayErr
		}
		result = append(result, page...)
		if len(page) < pageSize {
			return result, nil
		}
		if page[len(page)-1].Sequence <= cursor {
			return nil, fmt.Errorf("Agent replay cursor did not advance")
		}
		cursor = page[len(page)-1].Sequence
	}
}

func (w *RunTurn) State(ctx context.Context, sessionID, userID string) (*client.RuntimeSession, error) {
	session, runtime, err := w.authorizedRuntime(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	return runtime.GetRuntimeSession(ctx, session.ID)
}

func (w *RunTurn) authorizedRuntime(ctx context.Context, sessionID, userID string) (*domain.Session, client.Runtime, error) {
	session, err := w.sessions.Authorize(ctx, sessionID, userID)
	if err != nil {
		return nil, nil, err
	}
	if w.routing != nil {
		runtime, err := w.routing.Facts(ctx, session)
		return session, runtime, err
	}
	runtime, ok := w.runtimes.Get(session.AgentType)
	if !ok {
		return nil, nil, fmt.Errorf("agent runtime %q is not registered", session.AgentType)
	}
	return session, runtime, nil
}

func (w *RunTurn) runtime(ctx context.Context, session *domain.Session, authToken string) (service.AgentContext, client.Runtime, error) {
	runtimeContext, err := w.sessions.RuntimeContext(ctx, session, authToken)
	if err != nil {
		return service.AgentContext{}, nil, err
	}
	if runtimeContext.AgentType != session.AgentType || runtimeContext.AgentID != session.AgentID || runtimeContext.ContextVersion != session.ContextVersion {
		return service.AgentContext{}, nil, domain.ErrContextChanged
	}
	if w.routing != nil {
		runtime, err := w.routing.Resolve(ctx, session)
		return runtimeContext, runtime, err
	}
	runtime, ok := w.runtimes.Get(session.AgentType)
	if !ok {
		return service.AgentContext{}, nil, fmt.Errorf("agent runtime %q is not registered", session.AgentType)
	}
	return runtimeContext, runtime, nil
}

func (w *RunTurn) AllowLive(ctx context.Context, id string, payload []byte) bool {
	if w.routing == nil {
		return true
	}
	guard, ok := w.routing.(interface {
		AllowLive(context.Context, string, []byte) bool
	})
	return ok && guard.AllowLive(ctx, id, payload)
}

func (w *RunTurn) ownerRuntime(ctx context.Context, id, user string) (*domain.Session, client.Runtime, error) {
	session, err := w.sessions.Authorize(ctx, id, user)
	if err != nil {
		return nil, nil, err
	}
	if w.routing != nil {
		runtime, err := w.routing.Owner(ctx, session)
		return session, runtime, err
	}
	return w.authorizedRuntime(ctx, id, user)
}
