package client

import (
	"context"
	"errors"
	"media_agent/hertz_infra/authn/appcredential"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ApplicationWorkspace validates the binding at its authoritative Control owner.
type ApplicationWorkspace struct{ BaseURL string }

func (c ApplicationWorkspace) Check(ctx context.Context, token, id string) error {
	if id == "" || c.BaseURL == "" {
		return appcredential.ErrDenied
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return appcredential.ErrUnavailable
	}
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.BaseURL, "/")+"/apps/v1/agent-workspaces/"+url.PathEscape(id), nil)
	if e != nil {
		return appcredential.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	h := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := h.Do(req)
	if e != nil {
		return appcredential.ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		return nil
	}
	if res.StatusCode == 401 || res.StatusCode == 403 || res.StatusCode == 404 {
		return appcredential.ErrDenied
	}
	return errors.Join(appcredential.ErrUnavailable, errors.New("application workspace check failed"))
}
