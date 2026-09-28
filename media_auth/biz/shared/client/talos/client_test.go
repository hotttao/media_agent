package talos

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientLifecycle(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer admin-secret" {
			t.Fatalf("authorization = %q", got)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == apiPrefix+"/issuedApiKeys":
			_, _ = w.Write([]byte(`{"issued_api_key":{"key_id":"key-1","actor_id":"Worker:1","status":"KEY_STATUS_ACTIVE","scopes":["workflow.worker.poll"],"expire_time":"2026-10-01T00:00:00Z"},"secret":"secret-once"}`))
		case r.Method == http.MethodPost && r.URL.Path == apiPrefix+"/apiKeys:verify":
			_, _ = w.Write([]byte(`{"is_valid":true,"key_id":"key-1","actor_id":"Worker:1","status":"KEY_STATUS_ACTIVE","scopes":["workflow.worker.poll"],"expire_time":"2026-10-01T00:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == apiPrefix+"/issuedApiKeys/key-1":
			_, _ = w.Write([]byte(`{"key_id":"key-1","actor_id":"Worker:1","status":"KEY_STATUS_ACTIVE","scopes":["workflow.worker.poll"]}`))
		case r.Method == http.MethodPost && r.URL.Path == apiPrefix+"/issuedApiKeys/key-1:rotate":
			_, _ = w.Write([]byte(`{"issued_api_key":{"key_id":"key-2","actor_id":"Worker:1","status":"KEY_STATUS_ACTIVE","scopes":["workflow.worker.poll"]},"secret":"rotated-once"}`))
		case r.Method == http.MethodPost && r.URL.Path == apiPrefix+"/issuedApiKeys/key-2:revoke":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL, AdminToken: "admin-secret"})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := client.Issue(context.Background(), IssueCommand{Name: "desktop", ActorID: "Worker:1", Scopes: []string{"workflow.worker.poll"}, TTL: "24h", RequestID: "req-1"})
	if err != nil || issued.KeyID != "key-1" || issued.Secret != "secret-once" {
		t.Fatalf("Issue() = %#v, %v", issued, err)
	}
	verified, err := client.Verify(context.Background(), issued.Secret)
	if err != nil || verified.ActorID != "Worker:1" || len(verified.Scopes) != 1 {
		t.Fatalf("Verify() = %#v, %v", verified, err)
	}
	if _, err := client.Get(context.Background(), issued.KeyID); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	rotated, err := client.Rotate(context.Background(), issued.KeyID, issued.Scopes)
	if err != nil || rotated.KeyID != "key-2" || rotated.Secret != "rotated-once" {
		t.Fatalf("Rotate() = %#v, %v", rotated, err)
	}
	if err := client.Revoke(context.Background(), rotated.KeyID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if len(calls) != 5 {
		t.Fatalf("calls = %v", calls)
	}
}

func TestVerifyMapsInvalidCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"is_valid":false,"error_code":"VERIFICATION_ERROR_REVOKED","error_message":"revoked"}`))
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL})
	_, err := client.Verify(context.Background(), "redacted-test-credential")
	if !errors.Is(err, ErrInvalidCredential) || strings.Contains(err.Error(), "redacted-test-credential") {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestClientTimeoutDoesNotLeakCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL, Timeout: 5 * time.Millisecond})
	_, err := client.Verify(context.Background(), "redacted-test-credential")
	if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "redacted-test-credential") {
		t.Fatalf("Verify() error = %v", err)
	}
}
