package workspace

import "context"

// AgentIdentity is a credential-free read projection, not a second identity
// aggregate. Configuration ID, unlike AgentID, identifies a stable Agent.
type AgentIdentity struct {
	ConfigurationID, WorkspaceID, OwnerUserID, OrganizationID, TeamID, WorkspaceState string
	OwnerAuthorized                                                                   bool
}

func (v AgentIdentity) ActorID() string { return "Agent:" + v.ConfigurationID }

type AgentIdentityReader interface {
	FindAgentIdentity(context.Context, string) (*AgentIdentity, error)
}
