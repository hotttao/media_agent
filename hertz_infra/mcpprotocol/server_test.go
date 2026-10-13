package mcpprotocol

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testCaller struct{}

func (testCaller) Call(_ context.Context, method string, _ []byte) ([]byte, bool, error) {
	if method == "tools/list" {
		return []byte(`{"tools":[{"name":"source__read","inputSchema":{"type":"object"}}],"ttlMs":100000,"cacheScope":"public"}`), true, nil
	}
	return nil, false, nil
}
func TestStandardProtocolSupportsLegacyAndCurrentClients(t *testing.T) {
	endpoint := httptest.NewServer(NewHandler(func(_ context.Context, h http.Header) (Caller, int, error) {
		if h.Get("X-Session-Token") != "stable" {
			return nil, 401, errors.New("denied")
		}
		return testCaller{}, 200, nil
	}))
	defer endpoint.Close()
	for _, version := range []string{"2025-03-26", "2025-11-25", "2026-07-28"} {
		t.Run(version, func(t *testing.T) {
			httpClient := &http.Client{Transport: headerTransport{}}
			peer, e := mcp.NewClient(&mcp.Implementation{Name: "compat", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: endpoint.URL, HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}, &mcp.ClientSessionOptions{ProtocolVersion: version})
			if e != nil {
				t.Fatal(e)
			}
			defer peer.Close()
			result, e := peer.ListTools(context.Background(), nil)
			if e != nil || len(result.Tools) != 1 {
				t.Fatal(result, e)
			}
			if result.TTLMs != 0 || result.CacheScope != "private" {
				t.Fatal("authorized discovery became cacheable", result.Cacheable)
			}
		})
	}
	request, _ := http.NewRequest("POST", endpoint.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Content-Type", "application/json")
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("unauthenticated request accepted", response.StatusCode)
	}
}

type headerTransport struct{}

func (headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header = req.Header.Clone()
	req.Header.Set("X-Session-Token", "stable")
	return http.DefaultTransport.RoundTrip(req)
}
