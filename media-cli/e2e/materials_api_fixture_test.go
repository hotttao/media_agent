package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
)

// Borrow a provisioned User API Token. Never log, rotate or revoke it.
type materialsAPIFixture struct {
	gateway, token, organization, user string
}

func newMaterialsAPIFixture(t *testing.T) materialsAPIFixture {
	t.Helper()
	f := materialsAPIFixture{gateway: os.Getenv("MEDIA_GATEWAY_ADDR"), token: os.Getenv("MEDIA_API_TOKEN"), organization: os.Getenv("MEDIA_E2E_ORGANIZATION_ID")}
	if f.gateway == "" || f.token == "" || f.organization == "" {
		t.Skip("configure unified gateway, provisioned API Token and test organization")
	}
	status, body := doJSONBearer(t, f.gateway, http.MethodGet, "/api/auth/me", f.token, nil)
	var user struct {
		ID string `json:"id"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &user) != nil || user.ID == "" {
		t.Fatalf("provisioned principal unavailable: status=%d", status)
	}
	f.user = user.ID
	return f
}

func materialsGatewayRequest(t *testing.T, addr, method, path, token string, body any) (int, []byte) {
	t.Helper()
	u, err := url.Parse(path)
	if err != nil {
		t.Fatal("invalid Materials request path")
	}
	query := u.Query()
	if !query.Has("organization_id") {
		query.Set("organization_id", os.Getenv("MEDIA_E2E_ORGANIZATION_ID"))
	}
	u.RawQuery = query.Encode()
	return doJSONBearer(t, addr, method, u.String(), token, body)
}

func (f materialsAPIFixture) request(t *testing.T, method, path string, body any, want int) []byte {
	t.Helper()
	status, result := materialsGatewayRequest(t, f.gateway, method, path, f.token, body)
	if status != want {
		t.Fatalf("%s %s status=%d want=%d", method, path, status, want)
	}
	return result
}

// This local HTTP adapter check is not a deployed authentication/business gate.
func TestMaterialsAPIClientContract(t *testing.T) {
	const credential = "synthetic-client-fixture"
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+credential || r.Header.Get("Cookie") != "" {
			t.Error("request did not use API Token exclusively")
		}
		switch r.URL.Path {
		case "/api/auth/me":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"verified-owner"}`))
		case "/api/v1/materials":
			if r.URL.Query().Get("organization_id") != "fixture-org" || r.URL.Query().Get("scope") != "ai_clothing" {
				t.Error("request lost organization or original query")
			}
		case "/api/v1/products":
			if r.URL.Query().Get("organization_id") != "other-org" {
				t.Error("explicit negative organization case overwritten")
			}
		default:
			t.Error("client attempted unexpected credential mutation")
		}
	}))
	defer srv.Close()
	t.Setenv("MEDIA_GATEWAY_ADDR", srv.URL)
	t.Setenv("MEDIA_API_TOKEN", credential)
	t.Setenv("MEDIA_E2E_ORGANIZATION_ID", "fixture-org")
	f := newMaterialsAPIFixture(t)
	if f.user != "verified-owner" {
		t.Fatal("fixture did not derive current principal")
	}
	f.request(t, "GET", "/api/v1/materials?scope=ai_clothing", nil, 200)
	f.request(t, "GET", "/api/v1/products?organization_id=other-org", nil, 200)
	if calls.Load() != 3 {
		t.Fatal("unexpected fixture request count")
	}
}
