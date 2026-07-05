// client_test.go：MCP server OAuth client 单测（httptest.Server 模拟）。
//
// 覆盖：DiscoverMetadata（支持 OAuth / 不支持 / 不可达）、ExchangeCode、RefreshToken（permanent vs 暂态）。
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domainmcp "media_agent/media_auth/biz/domain/mcp_oauth"
)

func TestDiscoverMetadata_SupportsOAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		if r.Header.Get("MCP-Protocol-Version") != "2025-03-26" {
			t.Errorf("missing MCP-Protocol-Version header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": "https://mcp.example.com/authorize",
			"token_endpoint":         "https://mcp.example.com/token",
		})
	}))
	defer srv.Close()

	c := NewClient(nil)
	meta, origin, ok, err := c.DiscoverMetadata(context.Background(), srv.URL+"/some/path")
	if err != nil || !ok {
		t.Fatalf("discover: ok=%v err=%v", ok, err)
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		t.Errorf("metadata empty: %+v", meta)
	}
	if origin != srv.URL {
		t.Errorf("origin: got %q, want %q", origin, srv.URL)
	}
}

func TestDiscoverMetadata_NotSupported(t *testing.T) {
	// 缺 token_endpoint → 不支持 OAuth。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"authorization_endpoint": "https://x/authorize"})
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, _, ok, err := c.DiscoverMetadata(context.Background(), srv.URL)
	if err != nil || ok {
		t.Errorf("missing token_endpoint: want ok=false, got ok=%v err=%v", ok, err)
	}
}

func TestDiscoverMetadata_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(nil)
	_, _, ok, _ := c.DiscoverMetadata(context.Background(), srv.URL)
	if ok {
		t.Errorf("404: want ok=false")
	}
}

func TestExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("grant_type: got %q", r.FormValue("grant_type"))
		}
		if r.FormValue("code") != "the-code" || r.FormValue("code_verifier") != "the-verifier" {
			t.Errorf("code/verifier missing")
		}
		json.NewEncoder(w).Encode(domainmcp.TokenResponse{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 1800})
	}))
	defer srv.Close()

	c := NewClient(nil)
	mc := domainmcp.McpClient{ClientID: "cid", Metadata: domainmcp.Metadata{TokenEndpoint: srv.URL}}
	resp, err := c.ExchangeCode(context.Background(), mc, "the-code", "the-verifier", "https://cb")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if resp.AccessToken != "at" || resp.RefreshToken != "rt" {
		t.Errorf("got resp=%+v", resp)
	}
}

func TestRefreshToken_Permanent(t *testing.T) {
	// 400 invalid_grant → permanent=true。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
	}))
	defer srv.Close()

	c := NewClient(nil)
	mc := domainmcp.McpClient{ClientID: "cid", Metadata: domainmcp.Metadata{TokenEndpoint: srv.URL}}
	_, permanent, err := c.RefreshToken(context.Background(), mc, "rt")
	if err == nil {
		t.Errorf("want error for 400")
	}
	if !permanent {
		t.Errorf("invalid_grant should be permanent")
	}
}

func TestRefreshToken_Transient(t *testing.T) {
	// 500 → permanent=false（暂态）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(nil)
	mc := domainmcp.McpClient{ClientID: "cid", Metadata: domainmcp.Metadata{TokenEndpoint: srv.URL}}
	_, permanent, _ := c.RefreshToken(context.Background(), mc, "rt")
	if permanent {
		t.Errorf("500 should be transient (permanent=false)")
	}
}

func TestRefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(domainmcp.TokenResponse{AccessToken: "new-at", ExpiresIn: 1800})
	}))
	defer srv.Close()

	c := NewClient(nil)
	mc := domainmcp.McpClient{ClientID: "cid", Metadata: domainmcp.Metadata{TokenEndpoint: srv.URL}}
	resp, permanent, err := c.RefreshToken(context.Background(), mc, "rt")
	if err != nil || permanent {
		t.Fatalf("success: err=%v permanent=%v", err, permanent)
	}
	if resp.AccessToken != "new-at" {
		t.Errorf("access: got %q", resp.AccessToken)
	}
}
