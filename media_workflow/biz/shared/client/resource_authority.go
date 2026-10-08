package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/media_workflow/biz/shared"
	"media_agent/media_workflow/biz/shared/auth"
	"net/http"
	"strings"
	"time"
)

// ResourceAuthorityClient deliberately has no membership cache. Failure is not
// equivalent to no membership, and JWT department claims are never consulted.
type ResourceAuthorityClient struct {
	baseURL, token string
	http           *http.Client
	control        *ControlClient
}

type templateResourceAuthority struct{ *ResourceAuthorityClient }

func (c templateResourceAuthority) Facts(ctx context.Context, user string) (shared.ResourceFacts, error) {
	return c.identityFacts(ctx, user)
}

// Enterprise templates do not depend on Control or existing Workspaces.
func (c *ResourceAuthorityClient) TemplateAuthority() shared.ResourceAuthority {
	return templateResourceAuthority{c}
}

func NewResourceAuthorityClient(baseURL, token string, control *ControlClient) *ResourceAuthorityClient {
	return &ResourceAuthorityClient{strings.TrimRight(baseURL, "/"), token, &http.Client{Timeout: 5 * time.Second}, control}
}

func (c *ResourceAuthorityClient) read(ctx context.Context, path string, input, result any) error {
	if c.baseURL == "" || c.token == "" {
		return shared.ErrResourceAuthorityUnavailable
	}
	body, err := json.Marshal(input)
	if err != nil {
		return shared.ErrResourceAuthorityUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return shared.ErrResourceAuthorityUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Auth-Internal-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return shared.ErrResourceAuthorityUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result) != nil {
		return shared.ErrResourceAuthorityUnavailable
	}
	return nil
}

type resourceTeamFacts struct {
	TeamID             string `json:"team_id"`
	OrganizationID     string `json:"organization_id"`
	TeamActive         bool   `json:"team_active"`
	OrganizationActive bool   `json:"organization_active"`
	TeamRole           string `json:"team_role"`
	OrganizationRole   string `json:"organization_role"`
	Allowed            bool   `json:"allowed"`
}

func (c *ResourceAuthorityClient) team(ctx context.Context, user, team, permission string) (resourceTeamFacts, error) {
	var facts resourceTeamFacts
	err := c.read(ctx, "/internal/team-authorization/check", map[string]string{"user_id": user, "team_id": team, "permission": permission}, &facts)
	if err == nil && (facts.TeamID != team || facts.OrganizationID == "") {
		err = shared.ErrResourceAuthorityUnavailable
	}
	return facts, err
}

func (c *ResourceAuthorityClient) identityFacts(ctx context.Context, user string) (shared.ResourceFacts, error) {
	var f shared.ResourceFacts
	if user == "" {
		return f, shared.ErrResourceAccessDenied
	}
	if p, ok := internaljwt.AgentPrincipalFromContext(ctx); ok && p.UserID != user {
		return f, shared.ErrResourceAccessDenied
	}
	var orgs struct {
		IDs []string `json:"organization_ids"`
	}
	if err := c.read(ctx, "/internal/organizations:byUser", map[string]string{"user_id": user}, &orgs); err != nil {
		return f, err
	}
	f.OrganizationIDs = orgs.IDs
	if p, ok := internaljwt.AgentPrincipalFromContext(ctx); ok {
		f.OrganizationIDs = nil
		if shared.HasResourceID(orgs.IDs, p.OrganizationID) {
			f.OrganizationIDs = []string{p.OrganizationID}
		}
	}
	if caller := auth.CallerFromContext(ctx); caller != nil && caller.OrganizationID != "" {
		f.OrganizationIDs = nil
		if shared.HasResourceID(orgs.IDs, caller.OrganizationID) {
			f.OrganizationIDs = []string{caller.OrganizationID}
		}
	}
	var teams struct {
		IDs []string `json:"team_ids"`
	}
	if err := c.read(ctx, "/internal/teams:byUser", map[string]string{"user_id": user}, &teams); err != nil {
		return f, err
	}
	for _, id := range teams.IDs {
		t, err := c.team(ctx, user, id, "use_workflow")
		if err != nil {
			return f, err
		}
		if !t.TeamActive || !t.OrganizationActive || t.TeamRole == "" || t.OrganizationRole == "" || !shared.HasResourceID(f.OrganizationIDs, t.OrganizationID) {
			continue
		}
		f.TeamIDs = append(f.TeamIDs, id)
		if t.Allowed {
			f.WorkflowTeamIDs = append(f.WorkflowTeamIDs, id)
			f.WorkflowOrganizationIDs = append(f.WorkflowOrganizationIDs, t.OrganizationID)
		}
	}
	return f, nil
}

func (c *ResourceAuthorityClient) Facts(ctx context.Context, user string) (shared.ResourceFacts, error) {
	f, err := c.identityFacts(ctx, user)
	if err != nil {
		return f, err
	}
	if c.control == nil {
		return f, shared.ErrResourceAuthorityUnavailable
	}
	workspaces, err := c.control.ListResourceWorkspaces(ctx, user)
	if err != nil {
		return f, shared.ErrResourceAuthorityUnavailable
	}
	for _, w := range workspaces {
		if p, ok := internaljwt.AgentPrincipalFromContext(ctx); ok && (p.WorkspaceID != w.ID || p.OrganizationID != w.OrganizationID) {
			continue
		}
		if shared.HasResourceID(f.OrganizationIDs, w.OrganizationID) && shared.HasResourceID(f.TeamIDs, w.TeamID) {
			f.WorkspaceIDs = append(f.WorkspaceIDs, w.ID)
		}
	}
	return f, nil
}

func (c *ResourceAuthorityClient) TeamOrganization(ctx context.Context, user, team string) (string, error) {
	t, err := c.team(ctx, user, team, "view")
	if err != nil {
		return "", err
	}
	if !t.TeamActive || !t.OrganizationActive || t.OrganizationRole == "" {
		return "", shared.ErrResourceAccessDenied
	}
	return t.OrganizationID, nil
}

func (c *ResourceAuthorityClient) GetWorkspaceForUser(ctx context.Context, id, user string) (*shared.ResourceWorkspace, error) {
	if c.control == nil {
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	w, err := c.control.GetWorkspaceForUser(ctx, id, user)
	if err != nil {
		if errors.Is(err, ErrWorkspaceReadDenied) || errors.Is(err, ErrWorkspaceDelegationDenied) {
			return nil, shared.ErrResourceAccessDenied
		}
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	if caller := auth.CallerFromContext(ctx); caller != nil && caller.OrganizationID != "" && caller.OrganizationID != w.OrganizationID {
		return nil, shared.ErrResourceAccessDenied
	}
	ok, err := c.control.CanUseWorkflow(ctx, id, user)
	if err != nil {
		return nil, shared.ErrResourceAuthorityUnavailable
	}
	if !ok {
		return nil, shared.ErrResourceAccessDenied
	}
	return &shared.ResourceWorkspace{ID: w.ID, OrganizationID: w.OrganizationID, TeamID: w.TeamID}, nil
}
