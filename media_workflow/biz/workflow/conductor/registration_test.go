package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"media_agent/media_workflow/biz/workflow/compiler"
)

func TestEnsureWorkflowVerifiesFrozenVersionWithoutBlindOverwrite(t *testing.T) {
	definition := compiler.Definition{Name: "frozen", Version: 3, SchemaVersion: 2, Tasks: []compiler.Task{{Name: "human", TaskReferenceName: "approval", Type: "HUMAN", InputParameters: map[string]any{"template": "frozen prompt"}}}}
	for _, scenario := range []string{"existing_same", "existing_conflict", "existing_task_extension", "existing_workflow_extension", "existing_schema_disabled", "new", "lost_committed_response", "lost_uncommitted_response", "unavailable_read", "changed_after_write", "unavailable_verification"} {
		t.Run(scenario, func(t *testing.T) {
			var actual *compiler.Definition
			if strings.HasPrefix(scenario, "existing") {
				copy := definition
				actual = &copy
			}
			if scenario == "existing_conflict" {
				actual.Description = "different"
			}
			gets, puts := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					gets++
					if r.URL.Path != "/api/metadata/workflow/frozen" || r.URL.Query().Get("version") != "3" {
						t.Error("did not query exact frozen version")
					}
					if scenario == "unavailable_read" || scenario == "unavailable_verification" && gets > 1 {
						w.WriteHeader(503)
						_, _ = w.Write([]byte("private upstream body"))
						return
					}
					if actual == nil {
						w.WriteHeader(404)
						return
					}
					data, _ := json.Marshal(actual)
					var decorated map[string]any
					_ = json.Unmarshal(data, &decorated)
					decorated["createTime"] = 123
					decorated["updateTime"] = 456
					if scenario == "existing_task_extension" {
						decorated["tasks"].([]any)[0].(map[string]any)["asyncComplete"] = true
					}
					if scenario == "existing_workflow_extension" {
						decorated["variables"] = map[string]any{"unexpected": "execution input"}
					}
					if scenario == "existing_schema_disabled" {
						decorated["enforceSchema"] = false
					}
					_ = json.NewEncoder(w).Encode(decorated)
				case http.MethodPut:
					puts++
					var defs []compiler.Definition
					if json.NewDecoder(r.Body).Decode(&defs) != nil || len(defs) != 1 {
						t.Error("invalid register request")
						w.WriteHeader(400)
						return
					}
					if scenario != "lost_uncommitted_response" {
						actual = &defs[0]
					}
					if scenario == "changed_after_write" {
						actual.Description = "modified after write"
					}
					if strings.HasPrefix(scenario, "lost_") {
						w.WriteHeader(503)
						_, _ = w.Write([]byte("secret upstream payload"))
						return
					}
					_, _ = w.Write([]byte(`{"bulkErrorResults":{}}`))
				default:
					t.Error("unexpected write method")
					w.WriteHeader(400)
				}
			}))
			defer server.Close()
			err := NewClient(server.URL).EnsureWorkflow(context.Background(), definition)
			switch scenario {
			case "existing_same", "new", "lost_committed_response":
				if err != nil {
					t.Fatal(err)
				}
			case "existing_conflict", "existing_task_extension", "existing_workflow_extension", "existing_schema_disabled", "changed_after_write":
				if !errors.Is(err, ErrWorkflowDefinitionConflict) {
					t.Fatal("conflict not detected", err)
				}
			default:
				if !errors.Is(err, ErrWorkflowRegistrationUnavailable) {
					t.Fatal("unverified registration accepted", err)
				}
			}
			wantPuts := 1
			if strings.HasPrefix(scenario, "existing") || scenario == "unavailable_read" {
				wantPuts = 0
			}
			if puts != wantPuts || gets < 1 {
				t.Fatal("blind registration or repeat write")
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Fatal("raw upstream body exposed")
			}
		})
	}
}

func TestConductorHTTPErrorDoesNotExposeBodyOrPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte("secret result"))
	}))
	defer server.Close()
	err := NewClient(server.URL).doJSON(context.Background(), http.MethodGet, "/private?token=secret", nil, nil)
	var status *HTTPStatusError
	if !errors.As(err, &status) || status.StatusCode != 503 || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
		t.Fatal("unsafe HTTP error")
	}
}
