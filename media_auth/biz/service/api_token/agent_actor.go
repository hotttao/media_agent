package apitoken

import (
	"context"
	"errors"
	"strings"

	"media_agent/media_auth/biz/shared/client/control"
)

type AgentAuthority interface {
	ResolveAgentIdentity(context.Context, string) (control.AgentIdentity, error)
}

// AgentActorValidator never caches authority. Identity facts are rechecked by
// existing Create, Verify and Rotate paths, independently of Talos key validity.
type AgentActorValidator struct{ authority AgentAuthority }

func NewAgentActorValidator(authority AgentAuthority) *AgentActorValidator {
	return &AgentActorValidator{authority: authority}
}

func (v *AgentActorValidator) Validate(ctx context.Context, actor, owner, organization string) bool {
	valid, err := v.Assess(ctx, actor, owner, organization)
	return err == nil && valid
}

// Assess separates authoritative invalidation from failure to obtain facts.
// Only (false, nil) is evidence usable by a revocation workflow. A lookup
// outage, malformed authority response or cancellation must never be used to
// revoke a credential. Interactive authentication still fails closed via Validate.
func (v *AgentActorValidator) Assess(ctx context.Context, actor, owner, organization string) (bool, error) {
	id, ok := strings.CutPrefix(actor, "Agent:")
	if !ok || id == "" || owner == "" || organization == "" {
		return false, nil
	}
	if v == nil || v.authority == nil || ctx.Err() != nil {
		return false, control.ErrUnavailable
	}
	facts, err := v.authority.ResolveAgentIdentity(ctx, id)
	if ctx.Err() != nil {
		return false, control.ErrUnavailable
	}
	if errors.Is(err, control.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, control.ErrUnavailable
	}
	if facts.ConfigurationID != id || facts.ActorID != actor || facts.WorkspaceID == "" || facts.TeamID == "" || facts.OwnerUserID == "" || facts.OrganizationID == "" {
		return false, control.ErrUnavailable
	}
	switch facts.WorkspaceState {
	case "deleted":
		return false, nil
	case "running", "stopped":
	default:
		return false, control.ErrUnavailable
	}
	return facts.OwnerUserID == owner && facts.OrganizationID == organization && facts.OwnerAuthorized, nil
}
