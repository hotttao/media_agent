// Package resourceaccess reads current identity facts; resource owners decide ACLs.
package resourceaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Team struct {
	TeamID             string `json:"team_id"`
	OrganizationID     string `json:"organization_id"`
	TeamRole           string `json:"team_role"`
	OrganizationRole   string `json:"organization_role"`
	TeamActive         bool   `json:"team_active"`
	OrganizationActive bool   `json:"organization_active"`
	Allowed            bool   `json:"allowed"`
}
type Facts struct {
	Member           bool
	OrganizationRole string
	TeamIDs          []string
}
type Client struct {
	baseURL, token string
	http           *http.Client
}

func New(baseURL, token string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{strings.TrimRight(baseURL, "/"), token, client}
}
func (c *Client) Resolve(ctx context.Context, user, organization string) (Facts, error) {
	var organizations struct {
		IDs           []string `json:"organization_ids"`
		Organizations []struct {
			ID     string `json:"organization_id"`
			Role   string `json:"role"`
			Status string `json:"status"`
		} `json:"organizations"`
	}
	if err := c.post(ctx, "/internal/organizations:byUser", map[string]string{"user_id": user}, &organizations); err != nil {
		return Facts{}, err
	}
	facts := Facts{TeamIDs: []string{}}
	for _, id := range organizations.IDs {
		if id == organization {
			facts.Member = true
			break
		}
	}
	for _, row := range organizations.Organizations {
		if row.ID == organization && row.Status == "active" && facts.Member {
			facts.OrganizationRole = row.Role
		}
	}
	if !facts.Member {
		return facts, nil
	}
	var teams struct {
		IDs []string `json:"team_ids"`
	}
	if err := c.post(ctx, "/internal/teams:byUser", map[string]string{"user_id": user}, &teams); err != nil {
		return Facts{}, err
	}
	for _, id := range teams.IDs {
		team, err := c.Team(ctx, user, id)
		if err != nil {
			return Facts{}, err
		}
		if team.OrganizationID == organization && team.TeamActive && team.OrganizationActive && team.TeamRole != "" && team.Allowed {
			facts.TeamIDs = append(facts.TeamIDs, id)
		}
	}
	return facts, nil
}
func (c *Client) Team(ctx context.Context, user, teamID string) (Team, error) {
	var result Team
	err := c.post(ctx, "/internal/team-authorization/check", map[string]string{"user_id": user, "team_id": teamID, "permission": "view"}, &result)
	if err == nil && result.TeamID != teamID {
		return Team{}, fmt.Errorf("authorization team mismatch")
	}
	return result, err
}
func (c *Client) post(ctx context.Context, path string, payload any, result any) error {
	if c.baseURL == "" || c.token == "" {
		return fmt.Errorf("identity fact service is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Auth-Internal-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("identity facts unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("identity facts unavailable: status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result)
}
