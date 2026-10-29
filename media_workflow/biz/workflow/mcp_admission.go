package workflow

import (
	"context"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/hertz_infra/authn/resourceaccess"
	policy "media_agent/media_workflow/biz/policy/mcp_access"
	"strings"
)

type MCPAdmission struct {
	Verifier     *internaljwt.Verifier
	ServiceToken string
	Facts        *resourceaccess.Client
	Workspaces   resourceaccess.WorkspaceClient
}

func (a MCPAdmission) Authenticate(ctx context.Context, authorization, presenter, method, path string) (mcpdelegation.Access, error) {
	raw, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok || !mcpdelegation.ServiceAuthenticated(presenter, a.ServiceToken) {
		return mcpdelegation.Access{}, mcpdelegation.ErrInvalid
	}
	grant, err := mcpdelegation.Verify(ctx, a.Verifier, raw, mcpdelegation.Workflow)
	if err != nil || !policy.Allowed(grant.Operation, method, path) {
		return mcpdelegation.Access{}, mcpdelegation.ErrInvalid
	}
	if a.Facts == nil {
		return mcpdelegation.Access{}, mcpdelegation.ErrAuthorityUnavailable
	}
	facts, err := a.Facts.Resolve(ctx, grant.UserID, grant.OrganizationID)
	if err != nil {
		return mcpdelegation.Access{}, mcpdelegation.ErrAuthorityUnavailable
	}
	if !facts.Member {
		return mcpdelegation.Access{}, mcpdelegation.ErrInvalid
	}
	teams := facts.TeamIDs
	if grant.WorkspaceID != "" {
		w, err := a.Workspaces.Read(ctx, grant.UserID, grant.WorkspaceID)
		if err != nil {
			return mcpdelegation.Access{}, mcpdelegation.ErrAuthorityUnavailable
		}
		if w.OrganizationID != grant.OrganizationID || w.TeamID == "" {
			return mcpdelegation.Access{}, mcpdelegation.ErrInvalid
		}
		teams = []string{w.TeamID}
	}
	for _, id := range teams {
		t, err := a.Facts.TeamPermission(ctx, grant.UserID, id, "use_workflow")
		if err != nil {
			return mcpdelegation.Access{}, mcpdelegation.ErrAuthorityUnavailable
		}
		if t.Allowed && t.TeamActive && t.OrganizationActive && t.OrganizationID == grant.OrganizationID {
			return mcpdelegation.Access{Principal: grant.Principal, TeamIDs: facts.TeamIDs}, nil
		}
	}
	return mcpdelegation.Access{}, mcpdelegation.ErrInvalid
}
