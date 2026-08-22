package media_agents

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"media_agent/media_cli/cmd"
)

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
