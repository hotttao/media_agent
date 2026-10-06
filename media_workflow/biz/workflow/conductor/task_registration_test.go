package conductor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func taskMetadata(d TaskDefinition) map[string]any {
	encoded, _ := json.Marshal(normalizedTaskDefinition(d))
	var metadata map[string]any
	_ = json.Unmarshal(encoded, &metadata)
	metadata["createTime"] = float64(1)
	metadata["createdBy"] = "engine-audit"
	metadata["rateLimitFrequencyInSeconds"] = float64(1)
	metadata["backoffScaleFactor"] = float64(1)
	metadata["taskStatusListenerEnabled"] = true
	metadata["inputKeys"] = []any{}
	metadata["inputTemplate"] = map[string]any{}
	return metadata
}

func TestTaskDefinitionsFrozenRegistration(t *testing.T) {
	definition := TaskDefinition{Name: "frozen_v1", RetryCount: 2, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, TimeoutPolicy: "TIME_OUT_WF"}
	for _, tc := range []struct {
		name, mode string
		want       error
		posts      int32
	}{
		{"existing exact", "existing", nil, 0},
		{"existing retry conflict", "conflict", ErrTaskDefinitionConflict, 0},
		{"input template extension", "extension", ErrTaskDefinitionConflict, 0},
		{"listener disabled", "listener", ErrTaskDefinitionConflict, 0},
		{"explicit zero delay", "zero", ErrTaskDefinitionConflict, 0},
		{"backoff zero not default", "backoff", ErrTaskDefinitionConflict, 0},
		{"lookup unavailable", "unavailable", ErrTaskRegistrationUnavailable, 0},
		{"create verified", "create", nil, 1},
		{"committed response lost", "lost", nil, 1},
		{"post rejected uncommitted", "rejected", ErrTaskRegistrationUnavailable, 1},
		{"post returned success uncommitted", "uncommitted", ErrTaskRegistrationUnavailable, 1},
		{"post conflict observed", "changed", ErrTaskDefinitionConflict, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts, puts atomic.Int32
			var metadata map[string]any
			switch tc.mode {
			case "existing", "conflict", "extension", "listener", "zero", "backoff":
				metadata = taskMetadata(definition)
			}
			if tc.mode == "conflict" {
				metadata["retryCount"] = float64(9)
			}
			if tc.mode == "extension" {
				metadata["inputTemplate"] = map[string]any{"unfrozen": "value"}
			}
			if tc.mode == "listener" {
				metadata["taskStatusListenerEnabled"] = false
			}
			if tc.mode == "zero" {
				metadata["retryDelaySeconds"] = float64(0)
			}
			if tc.mode == "backoff" {
				metadata["backoffScaleFactor"] = float64(0)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PUT" {
					puts.Add(1)
					w.WriteHeader(500)
					return
				}
				if r.Method == "GET" && r.URL.Path == "/api/metadata/taskdefs/"+definition.Name {
					if tc.mode == "unavailable" {
						w.WriteHeader(503)
						return
					}
					if metadata == nil {
						w.WriteHeader(404)
						return
					}
					_ = json.NewEncoder(w).Encode(metadata)
					return
				}
				if r.Method == "POST" && r.URL.Path == "/api/metadata/taskdefs" {
					posts.Add(1)
					var received []TaskDefinition
					if json.NewDecoder(r.Body).Decode(&received) != nil || len(received) != 1 || received[0] != definition {
						w.WriteHeader(400)
						return
					}
					if tc.mode != "rejected" && tc.mode != "uncommitted" {
						metadata = taskMetadata(definition)
					}
					if tc.mode == "changed" {
						metadata["timeoutSeconds"] = float64(999)
					}
					if tc.mode == "lost" || tc.mode == "rejected" {
						w.WriteHeader(503)
						return
					}
					w.WriteHeader(204)
					return
				}
				w.WriteHeader(400)
			}))
			defer server.Close()
			api := NewClient(server.URL)
			err := api.RegisterTaskDefinitions(context.Background(), []TaskDefinition{definition})
			if !errors.Is(err, tc.want) || posts.Load() != tc.posts || puts.Load() != 0 {
				t.Fatal("registration outcome or write count incorrect")
			}
			if err == nil {
				if api.RegisterTaskDefinitions(context.Background(), []TaskDefinition{definition}) != nil || posts.Load() != tc.posts {
					t.Fatal("exact replay wrote metadata")
				}
			}
		})
	}
}

func TestTaskDefinitionsBatchPreflightAndMissingSubset(t *testing.T) {
	a := TaskDefinition{Name: "a", RetryCount: 1}
	b := TaskDefinition{Name: "b", RetryCount: 2}
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{true: "conflict before any write", false: "only missing subset"}[conflict], func(t *testing.T) {
			var writes atomic.Int32
			created := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					name := strings.TrimPrefix(r.URL.Path, "/api/metadata/taskdefs/")
					if name == "a" {
						m := taskMetadata(a)
						if conflict {
							m["retryCount"] = float64(99)
						}
						_ = json.NewEncoder(w).Encode(m)
						return
					}
					if name == "b" && created {
						_ = json.NewEncoder(w).Encode(taskMetadata(b))
						return
					}
					w.WriteHeader(404)
					return
				}
				writes.Add(1)
				var body []TaskDefinition
				if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body[0] != b {
					w.WriteHeader(400)
					return
				}
				created = true
				w.WriteHeader(204)
			}))
			defer server.Close()
			err := NewClient(server.URL).RegisterTaskDefinitions(context.Background(), []TaskDefinition{b, a, a})
			if conflict {
				if !errors.Is(err, ErrTaskDefinitionConflict) || writes.Load() != 0 {
					t.Fatal("batch wrote before resolving conflict")
				}
			} else if err != nil || writes.Load() != 1 {
				t.Fatal("batch failed to create only missing definition")
			}
		})
	}
}

func TestTaskDefinitionsRejectDuplicateNameConflictBeforeNetwork(t *testing.T) {
	api := NewClient("http://invalid.example")
	if err := api.RegisterTaskDefinitions(context.Background(), []TaskDefinition{{Name: "same", RetryCount: 1}, {Name: "same", RetryCount: 2}}); !errors.Is(err, ErrTaskDefinitionConflict) {
		t.Fatal("contradictory task contracts accepted")
	}
}
