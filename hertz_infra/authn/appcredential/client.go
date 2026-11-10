// Package appcredential implements the private, online application credential verification protocol.
package appcredential

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
)

var ErrDenied = errors.New("application access denied")
var ErrUnavailable = errors.New("application authority unavailable")

type Principal struct {
	ApplicationID  string   `json:"application_id"`
	UserID         string   `json:"user_id"`
	ExternalID     string   `json:"external_id"`
	TeamID         string   `json:"team_id"`
	OrganizationID string   `json:"organization_id"`
	Scopes         []string `json:"scopes"`
	ExpiresAt      int64    `json:"expires_at"`
}
type Client struct {
	base, secret string
	http         *http.Client
}

func New(base, secret string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), secret: secret, http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) Verify(ctx context.Context, raw, scope string) (Principal, error) {
	if c == nil || c.secret == "" {
		return Principal{}, ErrUnavailable
	}
	u, e := url.Parse(c.base)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Principal{}, ErrUnavailable
	}
	if !strings.HasPrefix(raw, "mac_") || len(raw) != 68 {
		return Principal{}, ErrDenied
	}
	body, _ := json.Marshal(map[string]string{"token": raw, "scope": scope})
	req, e := http.NewRequestWithContext(ctx, "POST", c.base+"/internal/apps/verify", bytes.NewReader(body))
	if e != nil {
		return Principal{}, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Media-Auth-Internal-Token", c.secret)
	res, e := c.http.Do(req)
	if e != nil {
		return Principal{}, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return Principal{}, ErrDenied
	}
	if res.StatusCode != 200 {
		return Principal{}, ErrUnavailable
	}
	var p Principal
	if json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&p) != nil {
		return Principal{}, ErrUnavailable
	}
	if p.ApplicationID == "" || p.UserID == "" || p.ExternalID == "" || p.TeamID == "" || p.OrganizationID == "" || p.ExpiresAt <= time.Now().Unix() {
		return Principal{}, ErrDenied
	}
	for _, s := range p.Scopes {
		if s == scope {
			return p, nil
		}
	}
	return Principal{}, ErrDenied
}
func Bearer(header string) string {
	kind, value, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(kind, "Bearer") {
		return ""
	}
	return value
}
