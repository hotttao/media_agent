package media_agents

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"media_agent/media_cli/cmd"
)

func TestSubmitTurnCommandSendsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/runtime-sessions/session-1/turns" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["idempotency_key"] != "request-1" || body["context_version"] != float64(2) {
			t.Fatalf("unexpected body: %+v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"turn_id":"turn-1","status":"running"}`))
	}))
	defer server.Close()
	oldHost := cmd.FlagHost
	cmd.FlagHost = server.URL
	defer func() { cmd.FlagHost = oldHost }()
	submitTurnSessionID, submitTurnKey, submitTurnContext, submitTurnInput = "session-1", "request-1", 2, `{"text":"hello"}`
	submitTurnCmd.SetOut(&bytes.Buffer{})
	if err := submitTurnCmd.RunE(submitTurnCmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRequestAndPrintRuntimeSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/runtime-sessions/session-1" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"session-1"}`))
	}))
	defer server.Close()
	oldHost := cmd.FlagHost
	cmd.FlagHost = server.URL
	defer func() { cmd.FlagHost = oldHost }()
	output := &bytes.Buffer{}
	command := getRuntimeSessionCmd
	command.SetOut(output)

	if err := requestAndPrint(command, http.MethodGet, "/api/v1/runtime-sessions/session-1", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"session_id":"session-1"`) {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestRequestAndPrintRuntimeWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/runtime-workspace" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"workspace_id":"workspace-1","storage_id":"storage-1"}`))
	}))
	defer server.Close()
	oldHost := cmd.FlagHost
	cmd.FlagHost = server.URL
	defer func() { cmd.FlagHost = oldHost }()
	output := &bytes.Buffer{}
	getRuntimeWorkspaceCmd.SetOut(output)

	if err := requestAndPrint(getRuntimeWorkspaceCmd, http.MethodGet, "/api/v1/runtime-workspace", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"storage_id":"storage-1"`) {
		t.Fatalf("unexpected output: %s", output.String())
	}
}
