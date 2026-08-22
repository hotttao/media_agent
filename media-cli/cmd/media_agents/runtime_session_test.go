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
	t.Setenv("MEDIA_AGENT_API_KEY", "secret-from-environment")
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
		if body["api_key"] != "secret-from-environment" {
			t.Fatalf("API key not forwarded")
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"turn_id":"turn-1","status":"running"}`))
	}))
	defer server.Close()
	oldHost := cmd.FlagHost
	cmd.FlagHost = server.URL
	defer func() { cmd.FlagHost = oldHost }()
	submitTurnSessionID, submitTurnKey, submitTurnContext, submitTurnInput = "session-1", "request-1", 2, `{"text":"hello"}`
	output := &bytes.Buffer{}
	submitTurnCmd.SetOut(output)
	if err := submitTurnCmd.RunE(submitTurnCmd, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "secret-from-environment") {
		t.Fatal("API key leaked to command output")
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

func TestGetAndInterruptTurnCommands(t *testing.T) {
	requests := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
		}
		_, _ = w.Write([]byte(`{"turn_id":"turn-1"}`))
	}))
	defer server.Close()
	oldHost := cmd.FlagHost
	cmd.FlagHost = server.URL
	defer func() { cmd.FlagHost = oldHost }()
	getTurnSessionID, getTurnID = "session-1", "turn-1"
	interruptTurnSessionID, interruptTurnID = "session-1", "turn-1"
	getTurnCmd.SetOut(&bytes.Buffer{})
	interruptTurnCmd.SetOut(&bytes.Buffer{})
	if err := getTurnCmd.RunE(getTurnCmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := interruptTurnCmd.RunE(interruptTurnCmd, nil); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /api/v1/runtime-sessions/session-1/turns/turn-1",
		"POST /api/v1/runtime-sessions/session-1/turns/turn-1/interrupt",
	}
	if len(requests) != len(want) || requests[0] != want[0] || requests[1] != want[1] {
		t.Fatalf("unexpected requests: %v", requests)
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
