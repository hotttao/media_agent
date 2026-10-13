package container_workload_test

import (
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	contract "media_agent/hertz_infra/agentworkload"
	handler "media_agent/media_runtime_controller/biz/handler/container_workload"
	"media_agent/media_runtime_controller/biz/middleware"
	router "media_agent/media_runtime_controller/biz/router/container_workload"
	native "media_agent/media_runtime_controller/docker"
	"strings"
	"testing"
)

func TestUnsupportedDockerOperationsReturn501BeforeEngineAccess(t *testing.T) {
	spec := contract.Spec{Driver: "docker", EnvironmentID: "env", WorkspaceID: "ws", AgentConfigID: "cfg", AgentType: "codex", Image: "alpine:3.20", Replicas: 1, Port: 3001, CPU: "100m", Memory: "64Mi", StorageSize: "1Gi", GatewayURL: "http://gateway:8006", SharedSecret: "shared"}
	name := contract.Name(spec.EnvironmentID, spec.WorkspaceID)
	spec.IdentitySecret = name + "-identity"
	spec.ClaimName = name + "-workspace"
	// No Engine exists: every unsupported operation must exit before native access.
	handler.Configure(&native.Service{})
	defer handler.Configure(nil)
	middleware.Configure("fixture-creation")
	h := server.New()
	router.Register(h)
	for _, test := range []struct {
		name     string
		replicas int32
		extra    map[string]any
	}{
		{"zero", 0, nil}, {"multiple", 2, nil},
		{"drain", 1, map[string]any{"drain_operation": "op"}},
		{"pending-count", 1, map[string]any{"pending_replicas": 1}},
		{"pending-version", 1, map[string]any{"pending_version": 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec.Replicas = test.replicas
			encoded, _ := json.Marshal(spec)
			body := map[string]any{"spec_json": string(encoded), "target_version": 1}
			for k, v := range test.extra {
				body[k] = v
			}
			raw, _ := json.Marshal(body)
			response := ut.PerformRequest(h.Engine, "POST", "/v1/workspaces/ws/container:ensure", &ut.Body{Body: strings.NewReader(string(raw)), Len: len(raw)}, ut.Header{Key: "Content-Type", Value: "application/json"}, ut.Header{Key: "X-Runtime-Creation-Credential", Value: "fixture-creation"}).Result()
			if response.StatusCode() != 501 || !strings.Contains(string(response.Body()), "not_implemented") {
				t.Fatalf("unexpected response %d %s", response.StatusCode(), response.Body())
			}
		})
	}
}
