package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchKratosIdentityReadsProfileButNotAdminMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/whoami" || r.Header.Get("Cookie") != "ory_session=valid" {
			t.Fatalf("request path=%q cookie=%q", r.URL.Path, r.Header.Get("Cookie"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"identity": {
				"id": "identity-1",
				"traits": {"email": "admin@example.test", "name": {"First": "Admin", "Last": "User"}},
				"metadata_admin": {"role": "platform_admin"}
			}
		}`))
	}))
	defer server.Close()

	id, email, name, err := fetchKratosIdentity(context.Background(), server.URL, "ory_session=valid")
	if err != nil {
		t.Fatal(err)
	}
	if id != "identity-1" || email != "admin@example.test" || name != "Admin User" {
		t.Fatalf("identity=%q email=%q name=%q", id, email, name)
	}
}

func TestFetchKratosPlatformAdminUsesAdminEndpoint(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"metadata_admin":{"role":"platform_admin"}}`))
	}))
	defer server.Close()

	platformAdmin, err := fetchKratosPlatformAdmin(context.Background(), server.URL, "identity-2")
	if err != nil {
		t.Fatal(err)
	}
	if !platformAdmin || path != "/admin/identities/identity-2" {
		t.Fatalf("platformAdmin=%v path=%q", platformAdmin, path)
	}
}
