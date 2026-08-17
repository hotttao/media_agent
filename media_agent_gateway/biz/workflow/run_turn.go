package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
	service "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
)

const turnTimeout = 24 * time.Hour
const toolResultCoalesceWindow = 300 * time.Millisecond

type RunTurn struct {
	sessions *service.Service
	runtimes client.Registry
	events   *sessionevents.Broadcaster
}

func NewRunTurn(sessions *service.Service, runtimes client.Registry, events *sessionevents.Broadcaster) *RunTurn {
	return &RunTurn{sessions: sessions, runtimes: runtimes, events: events}
}

// Start commits the Turn before dispatch and deliberately detaches Runtime
// execution from the initiating HTTP request. Subscribers observe progress via
// the Session event stream.
func (w *RunTurn) Start(ctx context.Context, sessionID, userID, authToken, key, message string) (*domain.Turn, error) {
	session, turn, err := w.sessions.StartTurn(ctx, sessionID, userID, key, message)
	if err != nil {
		return nil, err
	}
	runtimeContext, err := w.sessions.RuntimeContext(ctx, session, authToken)
	if err != nil {
		_ = w.sessions.FinishTurn(context.WithoutCancel(ctx), turn.ID, "failed")
		return nil, err
	}
	runtime, ok := w.runtimes.Get(session.AgentType)
	if !ok {
		_ = w.sessions.FinishTurn(context.WithoutCancel(ctx), turn.ID, "failed")
		return nil, fmt.Errorf("agent runtime %q is not registered", session.AgentType)
	}
	go w.execute(session, turn, runtimeContext, runtime, message)
	return turn, nil
}

func (w *RunTurn) Interrupt(ctx context.Context, sessionID, userID string) (*domain.Turn, error) {
	session, turn, err := w.sessions.RunningTurn(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}
	if session.RuntimeSessionID == "" {
		return nil, domain.ErrNotFound
	}
	runtime, ok := w.runtimes.Get(session.AgentType)
	if !ok {
		return nil, fmt.Errorf("agent runtime %q is not registered", session.AgentType)
	}
	interrupted, err := runtime.Interrupt(ctx, session.RuntimeSessionID)
	if err != nil {
		return nil, err
	}
	if !interrupted {
		return nil, domain.ErrNotFound
	}
	return turn, nil
}

func (w *RunTurn) execute(session *domain.Session, turn *domain.Turn, runtimeContext service.AgentContext, runtime client.Runtime, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	defer cancel()
	capabilities := client.PersistenceCapabilities(runtime)
	sink := newTurnEventSink(ctx, session.ID, turn.ID, w.sessions, w.events, capabilities.MaxToolResultBytes, toolResultCoalesceWindow)
	err := runtime.Chat(ctx, client.ChatInput{Message: message, SessionID: session.RuntimeSessionID, APIKey: runtimeContext.APIKey}, sink.Accept)
	if flushErr := sink.Flush(); flushErr != nil && err == nil {
		err = flushErr
	}
	status := "completed"
	reason := "completed"
	if sink.TerminalReason() == "interrupted" {
		status = "interrupted"
		reason = "interrupted"
	} else if err != nil {
		status = "failed"
		reason = "error"
		raw, _ := json.Marshal(map[string]any{"type": "error", "message": err.Error()})
		if persistErr := sink.persistAndPublish(raw); persistErr != nil {
			log.Printf("persist turn %s failure: runtime=%v persist=%v", turn.ID, err, persistErr)
		}
	}
	terminalRaw := sink.Terminal(reason)
	terminalEvent, finalizeErr := w.sessions.FinalizeTurn(context.Background(), session.ID, turn.ID, status, terminalRaw)
	if finalizeErr != nil {
		log.Printf("finalize turn %s: %v", turn.ID, finalizeErr)
		return
	}
	w.events.Publish(session.ID, sessionevents.Event{Sequence: terminalEvent.Sequence, Payload: append([]byte(nil), terminalRaw...)})
}
