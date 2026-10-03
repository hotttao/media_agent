package apitoken

import (
	"context"
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
	id, ok := strings.CutPrefix(actor, "Agent:")
	if !ok || id == "" || owner == "" || organization == "" || v == nil || v.authority == nil {
		return false
	}
	facts, err := v.authority.ResolveAgentIdentity(ctx, id)
	return err == nil && facts.ConfigurationID == id && facts.ActorID == actor && facts.WorkspaceID != "" && facts.TeamID != "" &&
		facts.OwnerUserID == owner && facts.OrganizationID == organization && facts.OwnerAuthorized &&
		(facts.WorkspaceState == "running" || facts.WorkspaceState == "stopped")
}
