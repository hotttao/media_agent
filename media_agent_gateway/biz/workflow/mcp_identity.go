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
	UserAssertion, OrganizationID, WorkspaceID                                     string
	SessionToken, AgentToken, InstanceCredential, ReplicaID, IncarnationID, TurnID string
	ExecutionEpoch                                                                 int64
}
type MCPContext = client.MCPScope
type MCPIdentity struct {
	sessions    *sessions.Service
	credentials AgentCredentialVerifier
	authority   AgentTokenAuthority
	execution   *ExecutionContext
	users       MCPUserIdentity
}

func NewMCPIdentity(s *sessions.Service, c AgentCredentialVerifier, a AgentTokenAuthority, e *ExecutionContext) *MCPIdentity {
	return &MCPIdentity{sessions: s, credentials: c, authority: a, execution: e}
}

type MCPUserIdentity interface {
	Authenticate(context.Context, string, string, string) (MCPContext, error)
}

func (w *MCPIdentity) WithUsers(users MCPUserIdentity) *MCPIdentity { w.users = users; return w }
func (w *MCPIdentity) Authorize(ctx context.Context, c MCPCredentials) (MCPContext, error) {
	if c.UserAssertion != "" {
		if w == nil || w.users == nil {
			return MCPContext{}, ErrMCPUnavailable
		}
		if c.AgentToken != "" || c.SessionToken != "" || c.InstanceCredential != "" || c.ReplicaID != "" || c.IncarnationID != "" || c.TurnID != "" || c.ExecutionEpoch != 0 {
			return MCPContext{}, ErrMCPForbidden
		}
		result, err := w.users.Authenticate(ctx, c.UserAssertion, c.OrganizationID, c.WorkspaceID)
		if errors.Is(err, client.ErrMCPUserDenied) {
			return MCPContext{}, ErrMCPForbidden
		}
		if err != nil {
			return MCPContext{}, ErrMCPUnavailable
		}
		return result, nil
	}
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
	return MCPContext{Kind: "managed_agent", ActorID: p.ActorID, CredentialID: p.CredentialID, OrganizationID: facts.OrganizationID, SessionID: s.ID, UserID: s.UserID, WorkspaceID: s.WorkspaceID, AgentConfigID: cfg, AgentType: s.AgentType, MetadataTargetContextVersion: target, ContextVersion: s.ContextVersion}, nil
}
