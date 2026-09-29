package talos

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func recoveryCommand() IssueCommand {
	return IssueCommand{Name: "failed-issuance", ActorID: "User:one", RequestID: "trusted-attempt",
		Scopes: []string{"api.access"}, Metadata: map[string]any{"api_token_id": "trusted-attempt", "issuance_id": "trusted-attempt"}}
}

func TestRecoverIssueAcceptsReplayAndDiscardsNewSecret(t *testing.T) {
	for _, hasSecret := range []bool{false, true} {
		t.Run(map[bool]string{false: "metadata-only replay", true: "new key secret discarded"}[hasSecret], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.URL.Path != apiPrefix+"/issuedApiKeys" ||
					request["request_id"] != "trusted-attempt" || request["actor_id"] != "User:one" {
					t.Error("recovery changed request identity")
				}
				metadata, ok := request["metadata"].(map[string]any)
				if !ok || metadata["api_token_id"] != "trusted-attempt" || metadata["issuance_id"] != "trusted-attempt" {
					t.Error("recovery metadata lost")
				}
				response := map[string]any{"issued_api_key": map[string]any{"key_id": "key", "actor_id": "User:one",
					"status": "KEY_STATUS_ACTIVE", "metadata": metadata}}
				if hasSecret {
					response["secret"] = "never-expose-this-secret"
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			client, _ := New(Config{BaseURL: server.URL})
			key, err := client.RecoverIssue(t.Context(), recoveryCommand())
			if err != nil || key.KeyID != "key" || key.ActorID != "User:one" {
				t.Fatal("could not resolve issuance metadata")
			}
			encoded, err := json.Marshal(key)
			if err != nil || strings.Contains(string(encoded), "never-expose-this-secret") {
				t.Fatal("recovery result leaked one-time credential")
			}
			if !hasSecret {
				if _, err := client.Issue(t.Context(), recoveryCommand()); err == nil {
					t.Fatal("normal issuance accepted missing one-time secret")
				}
			}
		})
	}
}

func TestRecoverIssueRejectsUntrustedRequestBeforeNetwork(t *testing.T) {
	for _, mutate := range []func(*IssueCommand){
		func(c *IssueCommand) { c.RequestID = "" },
		func(c *IssueCommand) { c.ActorID = " " },
		func(c *IssueCommand) { c.Name = "" },
		func(c *IssueCommand) { c.Metadata = nil },
		func(c *IssueCommand) { c.Metadata["api_token_id"] = "other" },
		func(c *IssueCommand) { c.Metadata["issuance_id"] = "other" },
		func(c *IssueCommand) { c.Metadata["issuance_id"] = []string{"trusted-attempt"} },
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("invalid recovery reached Talos")
		}))
		client, _ := New(Config{BaseURL: server.URL})
		cmd := recoveryCommand()
		mutate(&cmd)
		if _, err := client.RecoverIssue(t.Context(), cmd); err == nil {
			t.Fatal("invalid recovery identity accepted")
		}
		server.Close()
	}
}

func TestRecoverIssueRejectsOtherActorOrAttemptWithoutLeaking(t *testing.T) {
	for _, field := range []string{"key", "actor", "token", "attempt", "expiry", "missing metadata"} {
		t.Run(field, func(t *testing.T) {
			metadata := map[string]any{"api_token_id": "trusted-attempt", "issuance_id": "trusted-attempt"}
			key := map[string]any{"key_id": "key", "actor_id": "User:one", "metadata": metadata}
			const unsafe = "sensitive-upstream-value"
			switch field {
			case "key":
				key["key_id"] = ""
			case "actor":
				key["actor_id"] = unsafe
			case "token":
				metadata["api_token_id"] = unsafe
			case "attempt":
				metadata["issuance_id"] = unsafe
			case "expiry":
				key["expire_time"] = unsafe
			case "missing metadata":
				delete(key, "metadata")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"issued_api_key": key, "secret": unsafe})
			}))
			defer server.Close()
			client, _ := New(Config{BaseURL: server.URL})
			result, err := client.RecoverIssue(t.Context(), recoveryCommand())
			if err == nil || result.KeyID != "" || strings.Contains(err.Error(), unsafe) {
				t.Fatal("unsafe recovery response accepted or exposed")
			}
		})
	}
}

func TestRecoverIssueUnavailableKeepsErrorSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("sensitive-upstream-value"))
	}))
	defer server.Close()
	client, _ := New(Config{BaseURL: server.URL})
	key, err := client.RecoverIssue(t.Context(), recoveryCommand())
	if !errors.Is(err, ErrUnavailable) || key.KeyID != "" || strings.Contains(err.Error(), "sensitive-upstream-value") {
		t.Fatal("quota failure accepted or exposed")
	}
}
