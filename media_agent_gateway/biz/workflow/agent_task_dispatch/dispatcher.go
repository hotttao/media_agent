package agent_task_dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	sessiondomain "media_agent/media_agent_gateway/biz/domain/agent_session"
	taskdomain "media_agent/media_agent_gateway/biz/domain/agent_task"
	sessionservice "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type SessionService interface {
	Authorize(context.Context, string, string) (*sessiondomain.Session, error)
	RuntimeContext(context.Context, *sessiondomain.Session, string) (sessionservice.AgentContext, error)
	TransitionStatus(context.Context, *sessiondomain.Session, string) error
}

type Dispatcher struct {
	tasks    taskdomain.DispatchRepository
	outbox   taskdomain.OutboxRepository
	tx       dal.TxRunner
	sessions SessionService
	runtimes client.Registry
	now      func() time.Time
}

func NewDispatcher(tasks taskdomain.DispatchRepository, outbox taskdomain.OutboxRepository, tx dal.TxRunner, sessions SessionService, runtimes client.Registry) *Dispatcher {
	return &Dispatcher{tasks: tasks, outbox: outbox, tx: tx, sessions: sessions, runtimes: runtimes, now: func() time.Time { return time.Now().UTC() }}
}

func (d *Dispatcher) Process(ctx context.Context, action, taskID string) error {
	switch action {
	case "agent_task.cancel":
		return d.cancel(ctx, taskID)
	case "agent_task.dispatch":
		return d.dispatch(ctx, taskID)
	default:
		return fmt.Errorf("unsupported Agent Task action %q", action)
	}
}

func (d *Dispatcher) dispatch(ctx context.Context, taskID string) error {
	task, err := d.tasks.Claim(ctx, taskID, d.now())
	if errors.Is(err, taskdomain.ErrConflict) {
		current, getErr := d.tasks.Get(ctx, taskID)
		if getErr != nil {
			return getErr
		}
		if current.Status == taskdomain.StatusWaitingTurn {
			return d.reconcileTask(ctx, current)
		}
		if taskdomain.IsTerminal(current.Status) {
			return nil
		}
		return err
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(task.SessionID) == "" {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "session_required", "Agent Task requires a Session")
	}
	session, err := d.sessions.Authorize(ctx, task.SessionID, task.RequestedBy)
	if err != nil {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "session_unavailable", "Agent Session is unavailable")
	}
	if session.WorkspaceID != task.WorkspaceID {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "workspace_mismatch", "Agent Session workspace does not match Task")
	}
	runtimeContext, err := d.sessions.RuntimeContext(ctx, session, "")
	if err != nil {
		_ = d.tasks.MarkRetryWait(ctx, task.ID, "runtime_context_unavailable", "Runtime context is temporarily unavailable", d.now())
		return err
	}
	if runtimeContext.WorkspaceID != session.WorkspaceID || runtimeContext.AgentID != session.AgentID || runtimeContext.AgentType != session.AgentType || runtimeContext.ContextVersion != session.ContextVersion {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "context_conflict", "Agent Session context version no longer matches its Registration")
	}
	runtime, ok := d.runtimes.Get(session.AgentType)
	if !ok {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "runtime_unavailable", "Agent runtime is not registered")
	}
	registered, err := runtime.RegisterRuntimeSession(ctx, client.RegisterRuntimeSessionInput{
		SessionID: session.ID, ContextVersion: session.ContextVersion,
		ContextRef: fmt.Sprintf("workspace:%s/agent:%s/context:%d", session.WorkspaceID, session.AgentID, session.ContextVersion),
	})
	if err != nil {
		return d.submitFailure(ctx, task, err)
	}
	if registered.SessionID != session.ID || registered.ContextVersion != session.ContextVersion {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "registration_conflict", "Agent returned an incompatible Runtime Session")
	}
	if session.Status == sessiondomain.StatusStarting {
		if err := d.sessions.TransitionStatus(ctx, session, sessiondomain.StatusActive); err != nil {
			_ = d.tasks.MarkRetryWait(ctx, task.ID, "session_transition_failed", "Agent Session activation failed", d.now())
			return err
		}
	}
	if session.Status != sessiondomain.StatusActive {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "session_inactive", "Agent Session is inactive")
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(task.InputJSON), &input); err != nil {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "invalid_input", "Task input is invalid")
	}
	turn, err := runtime.SubmitTurn(ctx, client.SubmitTurnInput{SessionID: session.ID, IdempotencyKey: task.ID, ContextVersion: session.ContextVersion, Input: input, APIKey: runtimeContext.APIKey})
	if err != nil {
		return d.submitFailure(ctx, task, err)
	}
	if turn.TurnID == "" || turn.SessionID != session.ID || turn.IdempotencyKey != task.ID {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "turn_conflict", "Agent returned an incompatible Turn")
	}
	if err := d.tasks.MarkWaitingTurn(ctx, task.ID, turn.TurnID, d.now()); err != nil {
		if errors.Is(err, taskdomain.ErrCancelled) {
			return d.cancel(ctx, task.ID)
		}
		return err
	}
	task.Status, task.TurnID = taskdomain.StatusWaitingTurn, turn.TurnID
	return d.convergeTurn(ctx, task, turn)
}

func (d *Dispatcher) submitFailure(ctx context.Context, task *taskdomain.Task, err error) error {
	if client.IsDefinitiveRequestFailure(err) {
		return d.finish(ctx, task, taskdomain.StatusFailed, "", "agent_rejected", "Agent rejected Task submission")
	}
	if markErr := d.tasks.MarkRetryWait(ctx, task.ID, "agent_unavailable", "Agent submission is temporarily unavailable", d.now()); markErr != nil {
		return errors.Join(err, markErr)
	}
	return err
}

func (d *Dispatcher) Reconcile(ctx context.Context, limit int) error {
	tasks, err := d.tasks.ListWaitingTurn(ctx, limit)
	if err != nil {
		return err
	}
	var combined error
	for i := range tasks {
		if err := d.reconcileTask(ctx, &tasks[i]); err != nil {
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

func (d *Dispatcher) reconcileTask(ctx context.Context, task *taskdomain.Task) error {
	session, err := d.sessions.Authorize(ctx, task.SessionID, task.RequestedBy)
	if err != nil {
		return err
	}
	runtime, ok := d.runtimes.Get(session.AgentType)
	if !ok {
		return fmt.Errorf("agent runtime %q is not registered", session.AgentType)
	}
	turn, err := runtime.GetTurn(ctx, task.SessionID, task.TurnID)
	if err != nil {
		return err
	}
	return d.convergeTurn(ctx, task, turn)
}

func (d *Dispatcher) convergeTurn(ctx context.Context, task *taskdomain.Task, turn *client.Turn) error {
	switch strings.ToLower(turn.Status) {
	case "queued", "running":
		return nil
	case "completed":
		return d.finish(ctx, task, taskdomain.StatusCompleted, fmt.Sprintf(`{"turn_id":%q}`, turn.TurnID), "", "")
	case "failed", "interrupted", "cancelled":
		code := "turn_" + strings.ToLower(turn.Status)
		return d.finish(ctx, task, taskdomain.StatusFailed, fmt.Sprintf(`{"turn_id":%q}`, turn.TurnID), code, "Agent Turn ended without completion")
	default:
		return fmt.Errorf("unsupported Agent Turn status %q", turn.Status)
	}
}

func (d *Dispatcher) finish(ctx context.Context, task *taskdomain.Task, status taskdomain.Status, resultRef, code, message string) error {
	now := d.now()
	return d.tx.RunInTx(ctx, func(tx context.Context) error {
		if err := d.tasks.MarkTerminal(tx, task.ID, status, resultRef, code, message, now); err != nil {
			return err
		}
		eventType := "agent_task." + string(status)
		payload := fmt.Sprintf(`{"task_id":%q,"status":%q,"turn_id":%q}`, task.ID, status, task.TurnID)
		return d.outbox.Enqueue(tx, &taskdomain.OutboxEvent{AggregateID: task.ID, EventType: eventType, PayloadJSON: payload, AvailableAt: now, CreatedAt: now, UpdatedAt: now})
	})
}

func (d *Dispatcher) cancel(ctx context.Context, taskID string) error {
	task, err := d.tasks.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status != taskdomain.StatusCancelled || task.SessionID == "" || task.TurnID == "" {
		return nil
	}
	session, err := d.sessions.Authorize(ctx, task.SessionID, task.RequestedBy)
	if err != nil {
		return err
	}
	runtime, ok := d.runtimes.Get(session.AgentType)
	if !ok {
		return fmt.Errorf("agent runtime %q is not registered", session.AgentType)
	}
	turn, err := runtime.GetTurn(ctx, task.SessionID, task.TurnID)
	if err != nil {
		return err
	}
	switch strings.ToLower(turn.Status) {
	case "queued", "running":
		_, err = runtime.InterruptTurn(ctx, task.SessionID, task.TurnID)
		return err
	default:
		return nil
	}
}
