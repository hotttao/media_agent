package talos

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestUpstreamFailuresNeverExposeEchoedCredential(t *testing.T) {
	const secret = "credential-must-not-appear-in-errors"
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"server error", 503, secret},
		{"client error", 400, secret},
		{"invalid field type", 200, `{"is_valid":true,"scopes":"` + secret + `"}`},
		{"invalid expiry", 200, `{"is_valid":true,"expire_time":"` + secret + `"}`},
		{"verification error code", 200, `{"is_valid":false,"error_code":"` + secret + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, _ := New(Config{BaseURL: server.URL})
			_, err := client.Verify(t.Context(), secret)
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Fatal("upstream error was not safely redacted")
			}
			if tc.status == 503 && !errors.Is(err, ErrUnavailable) {
				t.Fatal("server failure did not map to unavailable")
			}
		})
	}
}

func TestRotateRejectsMissingReplacementSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"issued_api_key":{"key_id":"replacement","actor_id":"User:1"}}`))
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL})
	if _, err := client.Rotate(t.Context(), "old-key", []string{"api.access"}); err == nil {
		t.Fatal("empty replacement credential accepted")
	}
}

func TestListEncodesActorFilterAndCursorWithoutQueryInjection(t *testing.T) {
	actor := `User:one" OR status="anything`
	cursor := "cursor&filter=other+value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != apiPrefix+"/issuedApiKeys" || query.Get("filter") != "actor_id="+strconv.Quote(actor) || query.Get("page_token") != cursor || query.Get("page_size") != "100" {
			t.Error("actor or cursor changed query structure")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"issued_api_keys": []map[string]any{{"key_id": "key", "actor_id": actor, "metadata": map[string]any{"rotation_id": "operation"}}}, "next_page_token": "next"})
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL})
	page, err := client.List(t.Context(), actor, cursor)
	if err != nil || len(page.Keys) != 1 || page.NextPageToken != "next" || page.Keys[0].Metadata["rotation_id"] != "operation" {
		t.Fatal("key metadata or pagination lost")
	}
}

func TestRotatePreservesRecoveryMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Metadata map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Metadata["api_token_id"] != "token" || body.Metadata["rotation_id"] != "operation" {
			t.Error("rotation recovery identity not sent")
		}
		_, _ = w.Write([]byte(`{"issued_api_key":{"key_id":"new-key","actor_id":"User:one"},"secret":"one-time"}`))
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL})
	if _, err := client.RotateWithMetadata(t.Context(), "old-key", []string{"api.access"}, map[string]any{"api_token_id": "token", "rotation_id": "operation"}); err != nil {
		t.Fatal(err)
	}
}

func TestListRejectsInvalidOrCrossActorKeys(t *testing.T) {
	for _, body := range []string{`{"issued_api_keys":[{"actor_id":"User:one"}]}`, `{"issued_api_keys":[{"key_id":"key","actor_id":"User:other"}]}`, `{"issued_api_keys":"invalid"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		client, _ := New(Config{BaseURL: server.URL})
		_, err := client.List(t.Context(), "User:one", "")
		server.Close()
		if err == nil {
			t.Fatal("invalid list treated as successful recovery input")
		}
	}
}
