package mediaauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	domain "media_agent/media_control/biz/domain/workspace"
)

type authorizationResponse struct {
	Allowed           bool     `json:"allowed"`
	FactAllowed       bool     `json:"fact_allowed"`
	KetoAllowed       *bool    `json:"keto_allowed"`
	TeamID            string   `json:"team_id"`
	OrganizationID    string   `json:"organization_id"`
	TeamRole          string   `json:"team_role"`
	OrganizationRole  string   `json:"organization_role"`
	EffectiveFeatures []string `json:"effective_feature_keys"`
}

type Client struct {
	baseURL, internalToken string
	http                   *http.Client
}

func New(baseURL, internalToken string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), internalToken: internalToken, http: client}
}

func (c *Client) ListTeams(ctx context.Context, userID string) ([]string, error) {
	var response struct {
		TeamIDs []string `json:"team_ids"`
	}
	if err := c.post(ctx, "/internal/teams:byUser", map[string]string{"user_id": userID}, &response); err != nil {
		return nil, err
	}
	return response.TeamIDs, nil
}

func (c *Client) Authorize(ctx context.Context, userID, teamID, permission string) (domain.TeamAuthorization, error) {
	var response authorizationResponse
	err := c.post(ctx, "/internal/team-authorization/check", map[string]string{
		"user_id": userID, "team_id": teamID, "permission": permission,
	}, &response)
	return domain.TeamAuthorization{
		Allowed: response.Allowed, OrganizationID: response.OrganizationID, TeamID: response.TeamID,
		TeamRole: response.TeamRole, OrganizationRole: response.OrganizationRole, EffectiveFeatures: response.EffectiveFeatures,
	}, err
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	if c.baseURL == "" || c.internalToken == "" {
		return fmt.Errorf("media_auth internal client is not configured")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Auth-Internal-Token", c.internalToken)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("media_auth %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("media_auth %s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(message)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode media_auth %s: %w", path, err)
	}
	return nil
}
