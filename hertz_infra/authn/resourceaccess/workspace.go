package resourceaccess

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Workspace struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	TeamID         string `json:"team_id"`
}

// WorkspaceClient retrieves authoritative scope facts, not a permission cache.
type WorkspaceClient struct{ BaseURL, Credential string }

func (c WorkspaceClient) Read(ctx context.Context, user, id string) (Workspace, error) {
	if c.BaseURL == "" || c.Credential == "" || user == "" || id == "" {
		return Workspace{}, errors.New("workspace facts unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.BaseURL, "/")+"/api/v1/workspaces/"+url.PathEscape(id), nil)
	if err != nil {
		return Workspace{}, err
	}
	req.Header.Set("X-Control-Service-Token", c.Credential)
	req.Header.Set("X-Control-Acting-User-ID", user)
	res, err := (&http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return Workspace{}, errors.New("workspace facts unavailable")
	}
	defer res.Body.Close()
	var w Workspace
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&w) != nil || w.ID != id {
		return Workspace{}, errors.New("workspace facts unavailable")
	}
	return w, nil
}
