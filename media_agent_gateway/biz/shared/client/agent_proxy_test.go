package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"media_agent/hertz_infra/authn/internaljwt"
)

func TestAgentForwarderPreservesBoundRequest(t *testing.T) {
	var seen atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(true)
		body, _ := io.ReadAll(r.Body)
		if r.Method != "POST" || r.RequestURI != "/api/v1/jobs?limit=1" || string(body) != `{"workflow_id":"wf"}` || r.Header.Get("Authorization") != "Bearer assertion" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Cookie") != "" {
			t.Error("bound request changed or foreign credentials forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "upstream-secret")
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"conflict"}`))
	}))
	defer upstream.Close()
	f, err := NewAgentHTTPForwarder(upstream.URL, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.ForwardAgentRequest(context.Background(), AgentProxyRequest{Service: "workflow", Request: internaljwt.AgentRequest{Method: "POST", URI: "/api/v1/jobs?limit=1", ContentType: "application/json", Body: []byte(`{"workflow_id":"wf"}`)}}, "assertion")
	if err != nil || !seen.Load() || response.StatusCode != 409 || response.ContentType != "application/json" || string(response.Body) != `{"error":"conflict"}` {
		t.Fatal("upstream response not preserved", err)
	}
}
func TestAgentForwarderDoesNotFollowRedirects(t *testing.T) {
	var called atomic.Bool
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Store(true) }))
	defer outside.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, outside.URL, 307) }))
	defer upstream.Close()
	f, _ := NewAgentHTTPForwarder(upstream.URL, upstream.URL)
	response, err := f.ForwardAgentRequest(context.Background(), AgentProxyRequest{Service: "materials", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/materials"}}, "assertion")
	if err != nil || called.Load() || response.StatusCode != 307 {
		t.Fatal("redirect policy failed", err)
	}
}
func TestAgentForwarderBoundsResponseAndTargets(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", (8<<20)+1)))
	}))
	defer upstream.Close()
	f, _ := NewAgentHTTPForwarder(upstream.URL, upstream.URL)
	_, err := f.ForwardAgentRequest(context.Background(), AgentProxyRequest{Service: "materials", Request: internaljwt.AgentRequest{Method: "GET", URI: "/api/v1/materials"}}, "assertion")
	if !errors.Is(err, ErrAgentForwardUnavailable) {
		t.Fatal("oversized upstream response accepted")
	}
	for _, uri := range []string{"https://outside.test/api/v1/materials", "//outside.test/api/v1/materials", "/internal/token", "/api/v1/../token", "/api/v1/%6dater ial"} {
		_, err := f.ForwardAgentRequest(context.Background(), AgentProxyRequest{Service: "materials", Request: internaljwt.AgentRequest{Method: "GET", URI: uri}}, "assertion")
		if !errors.Is(err, ErrAgentForwardUnavailable) {
			t.Fatal("invalid target accepted")
		}
	}
	for _, address := range []string{"file:///tmp/service", "https://user:secret@host", "http://host/base", "http://host?query=1", "http://host#fragment"} {
		if _, err := NewAgentHTTPForwarder(address, upstream.URL); err == nil {
			t.Fatal("unsafe service root accepted")
		}
	}
}
