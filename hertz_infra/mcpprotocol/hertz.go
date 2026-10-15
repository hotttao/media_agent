package mcpprotocol

import (
	"bytes"
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"net/http"
)

// JSON-response mode requires no persistent stream on the Gateway side.
type protocolResponse struct {
	header    http.Header
	body      bytes.Buffer
	status    int
	oversized bool
}

func (r *protocolResponse) Header() http.Header { return r.header }
func (r *protocolResponse) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}
func (r *protocolResponse) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = 200
	}
	if r.body.Len()+len(body) > 16<<20 {
		r.oversized = true
		return len(body), nil
	}
	return r.body.Write(body)
}
func ServeHertz(ctx context.Context, c *app.RequestContext, handler http.Handler) {
	if len(c.Request.Body()) > 2<<20 {
		c.JSON(413, map[string]string{"error": "mcp_request_too_large"})
		return
	}
	req, e := http.NewRequestWithContext(ctx, string(c.Method()), string(c.URI().FullURI()), bytes.NewReader(c.Request.Body()))
	if e != nil {
		c.JSON(400, map[string]string{"error": "invalid_mcp_request"})
		return
	}
	if addr := c.RemoteAddr(); addr != nil {
		req.RemoteAddr = addr.String()
	}
	c.Request.Header.VisitAll(func(k, v []byte) { req.Header.Add(string(k), string(v)) })
	recorder := &protocolResponse{header: make(http.Header)}
	handler.ServeHTTP(recorder, req)
	if recorder.oversized {
		c.JSON(502, map[string]string{"error": "mcp_response_too_large"})
		return
	}
	for key, values := range recorder.header {
		for _, value := range values {
			c.Response.Header.Add(key, value)
		}
	}
	status := recorder.status
	if status == 0 {
		status = 200
	}
	c.SetStatusCode(status)
	c.Response.SetBody(recorder.body.Bytes())
}
