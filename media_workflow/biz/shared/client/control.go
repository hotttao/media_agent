package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ControlClient checks Workspace ownership and permissions at media_control.
// The acting user is carried by a service-authenticated header, never by the
// request body sent to media_control.
type ControlClient struct {
	baseURL string
	token   string
	http    *http.Client
}

type WorkspaceContext struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	TeamID         string `json:"team_id"`
	CreatedBy      string `json:"created_by"`
}

func NewControlClient(baseURL, serviceToken string) *ControlClient {
	return &ControlClient{baseURL: strings.TrimRight(baseURL, "/"), token: serviceToken, http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *ControlClient) GetWorkspaceForUser(ctx context.Context, workspaceID, userID string) (*WorkspaceContext, error) {
	if c.baseURL == "" || c.token == "" || workspaceID == "" || userID == "" {
		return nil, fmt.Errorf("workspace access client is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/workspaces/"+url.PathEscape(workspaceID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Control-Service-Token", c.token)
	req.Header.Set("X-Control-Acting-User-ID", userID)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get workspace: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get workspace: status %d", resp.StatusCode)
	}
	var result WorkspaceContext
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode workspace: %w", err)
	}
	if result.ID != workspaceID || result.OrganizationID == "" || result.TeamID == "" {
		return nil, fmt.Errorf("workspace context is incomplete")
	}
	return &result, nil
}

func (c *ControlClient) CanUseWorkflow(ctx context.Context, workspaceID, userID string) (bool, error) {
	if c.baseURL == "" || c.token == "" || workspaceID == "" || userID == "" {
		return false, fmt.Errorf("workspace access client is not configured")
	}
	input, _ := json.Marshal(map[string]string{"workspace_id": workspaceID, "action": "use_workflow"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/workspaces:check", bytes.NewReader(input))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Control-Service-Token", c.token)
	req.Header.Set("X-Control-Acting-User-ID", userID)
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("check workspace workflow access: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("check workspace workflow access: status %d", resp.StatusCode)
	}
	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode workspace workflow access: %w", err)
	}
	return result.Allowed, nil
}
