package workflow

import (
	"context"
	"errors"
	sessiondomain "media_agent/media_agent_gateway/biz/domain/agent_session"
	sessions "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
	"strings"
)

var ErrMCPForbidden = errors.New("MCP scope forbidden")
var ErrMCPUnavailable = errors.New("MCP scope unavailable")

type MCPCredentials struct {
	SessionToken, AgentToken, InstanceCredential, ReplicaID, IncarnationID, TurnID string
	ExecutionEpoch                                                                 int64
}
type MCPContext = client.MCPScope
type MCPIdentity struct {
	sessions    *sessions.Service
	credentials AgentCredentialVerifier
	authority   AgentTokenAuthority
	execution   *ExecutionContext
}

func NewMCPIdentity(s *sessions.Service, c AgentCredentialVerifier, a AgentTokenAuthority, e *ExecutionContext) *MCPIdentity {
	return &MCPIdentity{s, c, a, e}
}
func (w *MCPIdentity) Authorize(ctx context.Context, c MCPCredentials) (MCPContext, error) {
	if w == nil || w.sessions == nil || w.credentials == nil || w.authority == nil {
		return MCPContext{}, ErrMCPUnavailable
	}
	s, e := w.sessions.ResolveCredential(ctx, c.SessionToken)
	if e != nil {
		if errors.Is(e, sessiondomain.ErrForbidden) || errors.Is(e, sessiondomain.ErrNotFound) {
			return MCPContext{}, ErrMCPForbidden
		}
		return MCPContext{}, ErrMCPUnavailable
	}
	p, e := w.credentials.VerifyAgentCredential(ctx, c.AgentToken)
	if e != nil {
		if errors.Is(e, client.ErrAgentCredentialRejected) {
			return MCPContext{}, ErrMCPForbidden
		}
		return MCPContext{}, ErrMCPUnavailable
	}
	fixed, e := w.sessions.RuntimeContext(ctx, s, "")
	if e != nil {
		return MCPContext{}, ErrMCPUnavailable
	}
	cfg := s.AgentConfigID
	if cfg == "" {
		cfg = fixed.AgentConfigID
	}
	actor, ok := strings.CutPrefix(p.ActorID, "Agent:")
	if !ok || actor != cfg || p.UserID != s.UserID || p.CredentialID == "" || p.OrganizationID == "" || fixed.AgentConfigID != cfg || fixed.ContextVersion != s.ContextVersion || fixed.AgentType != s.AgentType || fixed.WorkspaceID != s.WorkspaceID {
		return MCPContext{}, ErrMCPForbidden
	}
	facts, e := w.authority.ResolveAgentIdentity(ctx, cfg)
	if e != nil {
		return MCPContext{}, ErrMCPUnavailable
	}
	if facts.ConfigurationID != cfg || facts.OwnerUserID != s.UserID || facts.WorkspaceID != s.WorkspaceID || facts.OrganizationID != p.OrganizationID || facts.ActorID != p.ActorID || !facts.OwnerAuthorized || facts.State != "running" {
		return MCPContext{}, ErrMCPForbidden
	}
	if s.EnvironmentID != "" {
		if w.execution == nil || c.ReplicaID == "" || c.IncarnationID == "" || c.TurnID == "" || c.ExecutionEpoch < 1 {
			return MCPContext{}, ErrMCPForbidden
		}
		if _, e = w.execution.Get(ctx, c.AgentToken, c.InstanceCredential, ExecutionContextCommand{WorkspaceID: s.WorkspaceID, AgentConfigID: cfg, ReplicaID: c.ReplicaID, IncarnationID: c.IncarnationID, SessionID: s.ID, TurnID: c.TurnID, ExecutionEpoch: c.ExecutionEpoch}); e != nil {
			if errors.Is(e, ErrMetadataForbidden) || errors.Is(e, client.ErrAgentCredentialRejected) {
				return MCPContext{}, ErrMCPForbidden
			}
			return MCPContext{}, ErrMCPUnavailable
		}
	}
	target := s.MetadataTargetContextVersion
	if target == "" {
		target = fixed.MetadataTargetContextVersion
	}
	return MCPContext{SessionID: s.ID, UserID: s.UserID, WorkspaceID: s.WorkspaceID, AgentConfigID: cfg, AgentType: s.AgentType, MetadataTargetContextVersion: target, ContextVersion: s.ContextVersion}, nil
}
