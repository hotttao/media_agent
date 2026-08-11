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

func (w *RunTurn) execute(session *domain.Session, turn *domain.Turn, runtimeContext service.AgentContext, runtime client.Runtime, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), turnTimeout)
	defer cancel()
	status := "failed"
	defer func() {
		if err := w.sessions.FinishTurn(context.Background(), turn.ID, status); err != nil {
			log.Printf("finish turn %s: %v", turn.ID, err)
		}
	}()
	err := runtime.Chat(ctx, client.ChatInput{Message: message, SessionID: session.RuntimeSessionID, APIKey: runtimeContext.APIKey}, func(raw json.RawMessage) error {
		event, err := w.sessions.PersistEvent(ctx, session.ID, turn.ID, raw)
		if err != nil {
			return err
		}
		w.events.Publish(session.ID, sessionevents.Event{Sequence: event.Sequence, Payload: append([]byte(nil), raw...)})
		return nil
	})
	if err != nil {
		raw, _ := json.Marshal(map[string]any{"type": "error", "message": err.Error()})
		event, persistErr := w.sessions.PersistEvent(context.Background(), session.ID, turn.ID, raw)
		if persistErr != nil {
			log.Printf("persist turn %s failure: runtime=%v persist=%v", turn.ID, err, persistErr)
			return
		}
		w.events.Publish(session.ID, sessionevents.Event{Sequence: event.Sequence, Payload: append([]byte(nil), raw...)})
		return
	}
	status = "completed"
}
