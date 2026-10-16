package workflow

import (
	"context"
	"errors"
	session "media_agent/media_agent_gateway/biz/domain/agent_session"
	sessions "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type ExecutionContext struct {
	registration *InstanceRegistration
	repo         session.BindingRepository
	sessions     *sessions.Service
	routing      *SessionRouting
}
type ExecutionContextCommand struct {
	WorkspaceID, AgentConfigID, ReplicaID, IncarnationID, SessionID, TurnID string
	ExecutionEpoch                                                          int64
}
type ExecutionCredentials struct {
	ModelProtocol, ModelName, ModelBaseURL string
	APIKey, SessionToken                   string
	CodexAuthJSON, ModelConnectionID       string
	ModelConnectionVersion                 int64
	ExecutionEpoch, ContextVersion         int64
}

func NewExecutionContext(reg *InstanceRegistration, repo session.BindingRepository, sessions *sessions.Service, routing *SessionRouting) *ExecutionContext {
	return &ExecutionContext{reg, repo, sessions, routing}
}
func (w *ExecutionContext) Get(ctx context.Context, token, credential string, cmd ExecutionContextCommand) (ExecutionCredentials, error) {
	if w == nil || w.registration == nil {
		return ExecutionCredentials{}, ErrMetadataUnavailable
	}
	instance, _, err := w.registration.VerifyCurrent(ctx, token, credential, cmd.WorkspaceID, cmd.AgentConfigID, cmd.ReplicaID, cmd.IncarnationID)
	if err != nil {
		return ExecutionCredentials{}, err
	}
	s, err := w.repo.GetSession(ctx, cmd.SessionID)
	if err != nil {
		return ExecutionCredentials{}, err
	}
	if s.WorkspaceID != cmd.WorkspaceID || s.AgentConfigID != cmd.AgentConfigID || s.EnvironmentID != instance.EnvironmentID || s.Status != session.StatusActive || cmd.ExecutionEpoch < 1 {
		return ExecutionCredentials{}, ErrMetadataForbidden
	}
	peer, err := w.routing.Facts(ctx, s)
	if err != nil {
		return ExecutionCredentials{}, err
	}
	facts, err := peer.GetRuntimeSession(ctx, s.ID)
	if err != nil {
		return ExecutionCredentials{}, err
	}
	if facts.ActiveTurnID != cmd.TurnID || facts.OwnerInstanceID != cmd.ReplicaID || facts.OwnerIncarnationID != cmd.IncarnationID || facts.ExecutionEpoch != cmd.ExecutionEpoch {
		return ExecutionCredentials{}, ErrMetadataForbidden
	}
	valid := s.BindingPhase == "active" && s.ReplicaID == cmd.ReplicaID && s.IncarnationID == cmd.IncarnationID && s.ExecutionEpoch == cmd.ExecutionEpoch
	if s.BindingPhase == "transferring" {
		valid = (s.ReplicaID == cmd.ReplicaID && s.IncarnationID == cmd.IncarnationID && s.ExpectedExecutionEpoch == cmd.ExecutionEpoch) || (facts.LastTransferID == s.TransferID && cmd.ExecutionEpoch == s.ExpectedExecutionEpoch+1)
	}
	if !valid {
		return ExecutionCredentials{}, ErrMetadataForbidden
	}
	turn, err := peer.GetTurn(ctx, s.ID, cmd.TurnID)
	if err != nil {
		return ExecutionCredentials{}, err
	}
	if turn.Status != "running" || turn.ExecutionEpoch != cmd.ExecutionEpoch {
		return ExecutionCredentials{}, ErrMetadataForbidden
	}
	fixed, err := w.sessions.RuntimeContext(ctx, s, "")
	if err != nil {
		if errors.Is(err, client.ErrModelAuthenticationRequired) {
			return ExecutionCredentials{}, err
		}
		return ExecutionCredentials{}, ErrMetadataUnavailable
	}
	if fixed.ContextVersion != s.ContextVersion || fixed.AgentConfigID != s.AgentConfigID {
		return ExecutionCredentials{}, ErrMetadataForbidden
	}
	sessionToken, err := w.sessions.EnsureToken(ctx, s, true)
	if err != nil || sessionToken == "" {
		return ExecutionCredentials{}, ErrMetadataUnavailable
	}
	return ExecutionCredentials{APIKey: fixed.APIKey, SessionToken: sessionToken, ExecutionEpoch: cmd.ExecutionEpoch, ContextVersion: s.ContextVersion, CodexAuthJSON: fixed.CodexAuthJSON, ModelConnectionID: fixed.ModelConnectionID, ModelConnectionVersion: fixed.ModelConnectionVersion, ModelProtocol: fixed.ModelProtocol, ModelName: fixed.ModelName, ModelBaseURL: fixed.ModelBaseURL}, nil
}
func (w *ExecutionContext) Resume(ctx context.Context, id, user, operation string, epoch int64) (*client.RuntimeSession, error) {
	s, err := w.sessions.Authorize(ctx, id, user)
	if err != nil {
		return nil, err
	}
	if _, err = w.sessions.RuntimeContext(ctx, s, ""); err != nil {
		return nil, err
	}
	runtime, err := w.routing.Resolve(ctx, s)
	if err != nil {
		return nil, err
	}
	if s.ExecutionEpoch != epoch {
		return nil, session.ErrBindingConflict
	}
	peer, ok := runtime.(client.ControlledRuntime)
	if !ok {
		return nil, ErrRuntimeWaiting
	}
	return peer.Resume(ctx, s.ID, operation)
}
