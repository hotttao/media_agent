package worker_identity

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	apiClient "media_agent/hertz_gen/media_auth/api_token_service"
	client "media_agent/hertz_gen/media_auth/worker_identity_service"
)

func TestCreateTokenPrivateDeliveryAndFailedWriteRevocation(t *testing.T) {
	for _, scenario := range []string{"success", "existing-output", "write-fails"} {
		t.Run(scenario, func(t *testing.T) {
			var created, revoked atomic.Int32
			const secret = "synthetic-credential-never-print"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/worker-identities/stable/api-tokens":
					created.Add(1)
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["name"] != "desktop" || body["idempotency_key"] != "once" {
						t.Error("CLI request fields lost")
					}
					w.WriteHeader(201)
					_, _ = w.Write([]byte(`{"api_token":{"id":"credential","actor_id":"Worker:stable"},"secret":"` + secret + `"}`))
				case "/api/api-tokens/credential/revoke":
					revoked.Add(1)
					_, _ = w.Write([]byte(`{"success":true}`))
				default:
					t.Errorf("unexpected route %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			oldCli, oldTokens := cli, tokens
			t.Cleanup(func() { cli, tokens = oldCli, oldTokens })
			var err error
			cli, err = client.NewWorkerIdentityServiceClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			tokens, err = apiClient.NewApiTokenServiceClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "credential.env")
			if scenario == "existing-output" {
				if err := os.WriteFile(path, []byte("preserve existing file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "write-fails" {
				path = filepath.Join(dir, "missing-parent", "credential.env")
			}
			command := createTokenCmd()
			out := &bytes.Buffer{}
			command.SetOut(out)
			command.SetErr(out)
			command.SetArgs([]string{"--id", "stable", "--name", "desktop", "--expires-at", "2026-09-30T00:00:00Z", "--idempotency-key", "once", "--env-out", path})
			err = command.Execute()
			if strings.Contains(out.String(), secret) {
				t.Fatal("Secret printed")
			}
			switch scenario {
			case "success":
				if err != nil || created.Load() != 1 || revoked.Load() != 0 {
					t.Fatal("private delivery failed", err)
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("Secret file permissions wrong")
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "MEDIA_API_TOKEN="+secret+"\n" {
					t.Fatal("private credential not delivered")
				}
			case "existing-output":
				if err == nil || created.Load() != 0 || revoked.Load() != 0 {
					t.Fatal("existing output did not prevent issuance")
				}
				data, _ := os.ReadFile(path)
				if string(data) != "preserve existing file" {
					t.Fatal("existing file overwritten")
				}
			case "write-fails":
				if err == nil || created.Load() != 1 || revoked.Load() != 1 {
					t.Fatal("failed private delivery left live credential")
				}
			}
		})
	}
}
