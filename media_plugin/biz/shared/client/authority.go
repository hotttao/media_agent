package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Authority struct {
	ControlURL, GatewayURL, AuthURL, ControlToken, MetadataCredential, AuthCredential string
	HTTP                                                                              *http.Client
}

func (a *Authority) request(ctx context.Context, base, method, path string, headers http.Header, in, out any) error {
	if base == "" {
		return d.ErrUnavailable
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return d.ErrUnavailable
	}
	req.Header = headers
	req.Header.Set("Content-Type", "application/json")
	client := a.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, err := client.Do(req)
	if err != nil {
		return d.ErrUnavailable
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 401, 403:
		return d.ErrForbidden
	case 404:
		return d.ErrNotFound
	case 409:
		return d.ErrConflict
	}
	if resp.StatusCode != 200 {
		return d.ErrUnavailable
	}
	if out != nil {
		if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
			return d.ErrUnavailable
		}
	}
	return nil
}
func (a *Authority) controlHeaders(user string) (http.Header, error) {
	if user == "" || a.ControlToken == "" {
		return nil, d.ErrUnavailable
	}
	return http.Header{"X-Control-Service-Token": {a.ControlToken}, "X-Control-Acting-User-ID": {user}}, nil
}
func (a *Authority) Workspace(ctx context.Context, user, id, permission string) (d.Workspace, error) {
	headers, err := a.controlHeaders(user)
	if err != nil {
		return d.Workspace{}, err
	}
	var access struct {
		Allowed bool `json:"allowed"`
	}
	err = a.request(ctx, a.ControlURL, "POST", "/internal/workspaces:check", headers, map[string]string{"workspace_id": id, "action": permission}, &access)
	if err != nil {
		return d.Workspace{}, err
	}
	if !access.Allowed {
		return d.Workspace{}, d.ErrForbidden
	}
	var value struct {
		ID             string `json:"id"`
		OrganizationID string `json:"organization_id"`
		AgentConfigID  string `json:"agent_configuration_id"`
	}
	err = a.request(ctx, a.ControlURL, "GET", "/api/v1/workspaces/"+url.PathEscape(id), headers, nil, &value)
	if err != nil {
		return d.Workspace{}, err
	}
	if value.ID != id || value.OrganizationID == "" || value.AgentConfigID == "" {
		return d.Workspace{}, d.ErrUnavailable
	}
	return d.Workspace{ID: value.ID, OrganizationID: value.OrganizationID, AgentConfigID: value.AgentConfigID}, nil
}
func (a *Authority) Definition(ctx context.Context, id string, version int64) (d.Definition, error) {
	if a.ControlToken == "" {
		return d.Definition{}, d.ErrUnavailable
	}
	var v struct {
		ID                 string `json:"definition_id"`
		OrganizationID     string `json:"organization_id"`
		Endpoint           string `json:"endpoint"`
		Transport          string `json:"transport"`
		AuthenticationType string `json:"authentication_type"`
		Status             string `json:"status"`
		Version            int64  `json:"version"`
	}
	query := url.Values{}
	if version > 0 {
		raw, _ := json.Marshal(version)
		query.Set("version", string(raw))
	}
	err := a.request(ctx, a.GatewayURL, "GET", "/internal/mcp-definitions/"+url.PathEscape(id)+"?"+query.Encode(), http.Header{"X-Control-Service-Token": {a.ControlToken}}, nil, &v)
	return d.Definition{ID: v.ID, OrganizationID: v.OrganizationID, Endpoint: v.Endpoint, Transport: v.Transport, AuthenticationType: v.AuthenticationType, Status: v.Status, Version: v.Version}, err
}
func (a *Authority) Release(ctx context.Context, user, ws, config, target string) (d.Release, error) {
	headers, err := a.controlHeaders(user)
	if err != nil {
		return d.Release{}, err
	}
	if a.MetadataCredential == "" {
		return d.Release{}, d.ErrUnavailable
	}
	headers.Set("X-Metadata-Gateway-Credential", a.MetadataCredential)
	query := url.Values{"agent_config_id": {config}, "target_context_version": {target}, "for_session": {"true"}}
	var v struct {
		WorkspaceID    string `json:"workspace_id"`
		AgentConfigID  string `json:"agent_config_id"`
		Target         string `json:"target_context_version"`
		ContextVersion int64  `json:"context_version"`
		Resources      []struct {
			Kind     string `json:"kind"`
			ID       string `json:"id"`
			Checksum string `json:"checksum"`
			Version  int64  `json:"version"`
		} `json:"resources"`
	}
	err = a.request(ctx, a.ControlURL, "GET", "/internal/workspaces/"+url.PathEscape(ws)+"/agent-metadata?"+query.Encode(), headers, nil, &v)
	if err != nil {
		return d.Release{}, err
	}
	result := d.Release{WorkspaceID: v.WorkspaceID, AgentConfigID: v.AgentConfigID, Target: v.Target, ContextVersion: v.ContextVersion}
	for _, ref := range v.Resources {
		result.Resources = append(result.Resources, d.Resource{Kind: ref.Kind, ID: ref.ID, Version: ref.Version, Checksum: ref.Checksum})
	}
	return result, nil
}
func (a *Authority) OAuthToken(ctx context.Context, user, origin string) (string, error) {
	if a.AuthCredential == "" {
		return "", d.ErrUnavailable
	}
	var result struct {
		AccessToken string `json:"access_token"`
		Status      string `json:"status"`
	}
	err := a.request(ctx, a.AuthURL, "POST", "/internal/mcp-oauth/token", http.Header{"X-MCP-Plugin-Credential": {a.AuthCredential}}, map[string]string{"user_id": user, "server_origin": origin}, &result)
	if err != nil {
		return "", err
	}
	if result.Status == "needs_reauth" {
		return "", d.ErrReauth
	}
	if result.Status != "connected" || result.AccessToken == "" {
		return "", d.ErrUnavailable
	}
	return result.AccessToken, nil
}
func (a *Authority) Authenticate(ctx context.Context, authorization, cookie string) (string, error) {
	var value struct {
		ID string `json:"id"`
	}
	err := a.request(ctx, a.AuthURL, "GET", "/api/auth/me", http.Header{"Authorization": {authorization}, "Cookie": {cookie}}, nil, &value)
	if err != nil {
		return "", err
	}
	if value.ID == "" {
		return "", d.ErrForbidden
	}
	return value.ID, nil
}
