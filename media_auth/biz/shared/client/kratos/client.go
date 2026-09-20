package kratos

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client exposes the small Kratos Admin API surface owned by media_auth.
// The admin endpoint must only be reachable on the internal auth network.
type Client struct {
	adminURL string
	http     *http.Client
}

func New(adminURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{adminURL: strings.TrimRight(adminURL, "/"), http: httpClient}
}

func (c *Client) RevokeIdentitySessions(ctx context.Context, identityID string) error {
	if c == nil || c.adminURL == "" || identityID == "" {
		return nil
	}
	endpoint := c.adminURL + "/admin/identities/" + url.PathEscape(identityID) + "/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("revoke Kratos identity sessions: status %d", resp.StatusCode)
	}
	return nil
}
