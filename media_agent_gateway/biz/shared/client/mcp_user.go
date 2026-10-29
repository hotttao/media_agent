package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
)

var ErrMCPUserDenied = errors.New("MCP User access denied")
var ErrMCPUserUnavailable = errors.New("MCP User authority unavailable")

// MCPUserAccess reads live Auth and Control facts. No positive permission cache.
type MCPUserAccess struct {
	Verifier                                               *internaljwt.Verifier
	AuthURL, AuthCredential, ControlURL, ControlCredential string
	HTTP                                                   *http.Client
}

func (c *MCPUserAccess) Authenticate(ctx context.Context, raw, organization, workspace string) (MCPScope, error) {
	if c == nil || c.Verifier == nil {
		return MCPScope{}, ErrMCPUserUnavailable
	}
	claims, err := c.Verifier.Verify(ctx, raw)
	if err != nil {
		return MCPScope{}, ErrMCPUserDenied
	}
	user, ok := strings.CutPrefix(claims.Subject, "User:")
	actor := ""
	if claims.Actor != nil {
		actor = claims.Actor.Sub
	}
	if !ok || claims.AuthSource != "api_token" || !hasScope(claims.Scope, "mcp.access") || actor != "" && actor != claims.Subject || organization == "" || claims.OrganizationID != "" && claims.OrganizationID != organization {
		return MCPScope{}, ErrMCPUserDenied
	}
	p := mcpdelegation.Principal{Kind: "user", UserID: user, ActorID: "User:" + user, CredentialID: claims.CredentialID, OrganizationID: organization, WorkspaceID: workspace}
	if !mcpdelegation.ValidPrincipal(p) {
		return MCPScope{}, ErrMCPUserDenied
	}
	scope := MCPScope{Kind: p.Kind, UserID: p.UserID, ActorID: p.ActorID, CredentialID: p.CredentialID, OrganizationID: p.OrganizationID, WorkspaceID: p.WorkspaceID, ExpiresAt: claims.ExpiresAt.Time}
	member, err := c.member(ctx, scope)
	if err != nil {
		return MCPScope{}, err
	}
	if !member {
		return MCPScope{}, ErrMCPUserDenied
	}
	if workspace != "" {
		if _, err = c.workspace(ctx, scope); err != nil {
			return MCPScope{}, err
		}
	}
	return scope, nil
}
func (c *MCPUserAccess) member(ctx context.Context, s MCPScope) (bool, error) {
	var out struct {
		IDs []string `json:"organization_ids"`
	}
	if err := c.request(ctx, false, "POST", "/internal/organizations:byUser", s, map[string]string{"user_id": s.UserID}, &out); err != nil {
		return false, err
	}
	for _, id := range out.IDs {
		if id == s.OrganizationID {
			return true, nil
		}
	}
	return false, nil
}
func (c *MCPUserAccess) workspace(ctx context.Context, s MCPScope) (string, error) {
	var out struct{ ID, OrganizationID, TeamID string }
	// Decode snake_case explicitly at the wire boundary.
	var wire struct {
		ID   string `json:"id"`
		Org  string `json:"organization_id"`
		Team string `json:"team_id"`
	}
	if err := c.request(ctx, true, "GET", "/api/v1/workspaces/"+url.PathEscape(s.WorkspaceID), s, nil, &wire); err != nil {
		return "", err
	}
	out.ID, out.OrganizationID, out.TeamID = wire.ID, wire.Org, wire.Team
	if out.ID != s.WorkspaceID || out.OrganizationID != s.OrganizationID || out.TeamID == "" {
		return "", ErrMCPUserDenied
	}
	return out.TeamID, nil
}
func (c *MCPUserAccess) ModuleAllowed(ctx context.Context, s MCPScope, module string) (bool, error) {
	if module != "workflow" && module != "materials" {
		return false, ErrMCPUserDenied
	}
	member, err := c.member(ctx, s)
	if err != nil || !member {
		return false, err
	}
	if s.WorkspaceID != "" {
		if _, err := c.workspace(ctx, s); err != nil {
			return false, err
		}
		var out struct {
			Allowed bool `json:"allowed"`
		}
		err := c.request(ctx, true, "POST", "/internal/workspaces:check", s, map[string]string{"workspace_id": s.WorkspaceID, "action": "use_" + module}, &out)
		return out.Allowed, err
	}
	var out struct {
		IDs []string `json:"team_ids"`
	}
	if err := c.request(ctx, false, "POST", "/internal/teams:byUser", s, map[string]string{"user_id": s.UserID}, &out); err != nil {
		return false, err
	}
	if len(out.IDs) > 1024 {
		return false, ErrMCPUserUnavailable
	}
	for _, id := range out.IDs {
		var facts struct {
			Org        string `json:"organization_id"`
			Allowed    bool   `json:"allowed"`
			TeamActive bool   `json:"team_active"`
			OrgActive  bool   `json:"organization_active"`
		}
		err := c.request(ctx, false, "POST", "/internal/team-authorization/check", s, map[string]string{"user_id": s.UserID, "team_id": id, "permission": "use_" + module}, &facts)
		if err != nil {
			return false, err
		}
		if facts.Org == s.OrganizationID && facts.Allowed && facts.TeamActive && facts.OrgActive {
			return true, nil
		}
	}
	return false, nil
}
func (c *MCPUserAccess) request(ctx context.Context, control bool, method, path string, s MCPScope, payload, out any) error {
	base, credential, header := c.AuthURL, c.AuthCredential, "X-Media-Auth-Internal-Token"
	if control {
		base, credential, header = c.ControlURL, c.ControlCredential, "X-Control-Service-Token"
	}
	if base == "" || credential == "" {
		return ErrMCPUserUnavailable
	}
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return ErrMCPUserUnavailable
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return ErrMCPUserUnavailable
	}
	req.Header.Set(header, credential)
	req.Header.Set("Content-Type", "application/json")
	if control {
		req.Header.Set("X-Control-Acting-User-ID", s.UserID)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, err := client.Do(req)
	if err != nil {
		return ErrMCPUserUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 403 || res.StatusCode == 404 {
		return ErrMCPUserDenied
	}
	if res.StatusCode != 200 {
		return ErrMCPUserUnavailable
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out) != nil {
		return ErrMCPUserUnavailable
	}
	return nil
}
