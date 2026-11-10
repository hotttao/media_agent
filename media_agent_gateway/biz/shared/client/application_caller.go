package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"media_agent/hertz_infra/authn/appcredential"
	"media_agent/hertz_infra/authn/applicationcaller"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ApplicationCaller struct{ BaseURL, Secret string }

func (c ApplicationCaller) Resolve(ctx context.Context, definition, endpoint string, scope MCPScope) (applicationcaller.Identity, bool, error) {
	if c.Secret == "" {
		return applicationcaller.Identity{}, false, appcredential.ErrUnavailable
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return applicationcaller.Identity{}, false, appcredential.ErrUnavailable
	}
	raw, _ := json.Marshal(map[string]string{"definition_id": definition, "endpoint": endpoint, "user_id": scope.UserID, "organization_id": scope.OrganizationID, "team_id": scope.TeamID})
	req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.BaseURL, "/")+"/internal/apps/caller", bytes.NewReader(raw))
	if e != nil {
		return applicationcaller.Identity{}, false, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Auth-Internal-Token", c.Secret)
	h := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := h.Do(req)
	if e != nil {
		return applicationcaller.Identity{}, false, appcredential.ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return applicationcaller.Identity{}, false, appcredential.ErrDenied
	}
	var p struct {
		Enabled       bool   `json:"enabled"`
		ApplicationID string `json:"application_id"`
		UserID        string `json:"user_id"`
		ExternalID    string `json:"external_id"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&p) != nil {
		return applicationcaller.Identity{}, false, appcredential.ErrUnavailable
	}
	if !p.Enabled {
		return applicationcaller.Identity{}, false, nil
	}
	if p.ApplicationID == "" || p.UserID != scope.UserID || p.ExternalID == "" {
		return applicationcaller.Identity{}, false, appcredential.ErrDenied
	}
	return applicationcaller.Identity{ApplicationID: p.ApplicationID, UserID: p.UserID, ExternalID: p.ExternalID, OrganizationID: scope.OrganizationID, TeamID: scope.TeamID, WorkspaceID: scope.WorkspaceID, SessionID: scope.SessionID}, true, nil
}
