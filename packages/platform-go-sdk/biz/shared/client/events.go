package client

import (
	"context"
	"errors"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Events is a transport extension: the generated JSON client buffers and decodes responses.
// SSE must retain ownership of its streaming body until the caller closes it.
func (c *Clients) Events(ctx context.Context, auth, id string, after int64) (*http.Response, error) {
	if id == "" || strings.ContainsAny(id, "/\\") || id == "." || id == ".." || after < 0 {
		return nil, errors.New("invalid event request")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", c.base+"/apps/v1/chat/sessions/"+url.PathEscape(id)+"/events?after_sequence="+strconv.FormatInt(after, 10), nil)
	if e != nil {
		return nil, errors.New("invalid event request")
	}
	req.Header.Set("Authorization", auth)
	res, e := c.stream.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("platform stream unavailable")
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, &contract.Error{StatusCode: res.StatusCode}
	}
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		res.Body.Close()
		return nil, errors.New("invalid event response")
	}
	return res, nil
}
