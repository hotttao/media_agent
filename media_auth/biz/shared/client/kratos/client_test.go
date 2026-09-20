package kratos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRevokeIdentitySessions(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := New(server.URL, server.Client()).RevokeIdentitySessions(context.Background(), "identity-1"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/admin/identities/identity-1/sessions" {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestRevokeIdentitySessionsRejectsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	if err := New(server.URL, server.Client()).RevokeIdentitySessions(context.Background(), "identity-1"); err == nil {
		t.Fatal("expected error")
	}
}
