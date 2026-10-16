package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	contract "media_agent/hertz_infra/agentworkload"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testPolicy(t *testing.T) (Policy, contract.Spec) {
	t.Helper()
	root := t.TempDir()
	keys := filepath.Join(root, "public-keys.json")
	if e := os.WriteFile(keys, []byte("{}"), 0600); e != nil {
		t.Fatal(e)
	}
	p := Policy{EnvironmentID: "test-env", Network: "test-private", Root: root, PublicKeysFile: keys, SharedSecret: "shared", Images: map[string]string{"codex": "alpine:3.20"}, DatabaseURL: "test-db", RabbitURL: "test-rabbit", AgentTokens: map[string]string{"config-1": "test-stable-token"}}
	s := contract.Spec{SessionExchange: "sessions.custom", MetadataExchange: "metadata.custom", Driver: "docker", EnvironmentID: p.EnvironmentID, WorkspaceID: "workspace-1", AgentConfigID: "config-1", AgentType: "codex", Image: p.Images["codex"], Replicas: 1, Port: 3001, CPU: "100m", Memory: "64Mi", StorageSize: "10Gi", GatewayURL: "http://gateway:8006", SharedSecret: p.SharedSecret}
	n := contract.Name(s.EnvironmentID, s.WorkspaceID)
	s.ClaimName = n + "-workspace"
	s.IdentitySecret = n + "-identity"
	return p, s
}
func TestConcurrentCreationStableProofAndRecovery(t *testing.T) {
	p, spec := testPolicy(t)
	var lock sync.Mutex
	var container *Inspection
	creates := 0
	failStart := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		defer lock.Unlock()
		path := strings.TrimPrefix(r.URL.Path, "/v1.45")
		switch {
		case r.Method == "GET" && strings.HasSuffix(path, "/json"):
			if container == nil {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(container)
		case path == "/containers/create":
			creates++
			var body struct {
				Image      string
				User       string
				Labels     map[string]string
				Env        []string
				HostConfig struct {
					NetworkMode   string
					RestartPolicy struct{ Name string }
					Mounts        []struct {
						Type, Source, Target string
						ReadOnly             bool
					}
				}
			}
			json.NewDecoder(r.Body).Decode(&body)
			container = &Inspection{ID: "native-id"}
			container.Config.User = body.User
			container.Config.Image = body.Image
			container.Config.Labels = body.Labels
			container.Config.Env = body.Env
			container.HostConfig.NetworkMode = body.HostConfig.NetworkMode
			container.HostConfig.RestartPolicy = body.HostConfig.RestartPolicy
			for _, mount := range body.HostConfig.Mounts {
				container.Mounts = append(container.Mounts, struct {
					Type, Source, Destination string
					RW                        bool
				}{mount.Type, mount.Source, mount.Target, !mount.ReadOnly})
			}
			json.NewEncoder(w).Encode(map[string]string{"Id": container.ID})
		case strings.HasSuffix(path, "/start"):
			if failStart {
				failStart = false
				w.WriteHeader(500)
				return
			}
			container.State.Running = true
			container.State.StartedAt = "2026-10-13T01:00:00.123Z"
			w.WriteHeader(204)
		case strings.Contains(path, "/stop"):
			container.State.Running = false
			container.State.FinishedAt = "2026-10-13T02:00:00Z"
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected Engine request %s %s", r.Method, path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	svc := Service{Engine: &Engine{HTTP: server.Client(), Base: server.URL + "/v1.45"}, Policy: p}
	ctx := context.Background()
	if _, e := svc.Ensure(ctx, State{Spec: spec, Version: 1}, "", "", false); e == nil {
		t.Fatal("lost start response not exposed")
	}
	if _, e := svc.Ensure(ctx, State{Spec: spec, Version: 1}, "", "", false); e != nil {
		t.Fatalf("same create retry did not repair start: %v", e)
	}
	v, e := svc.Ensure(ctx, State{Spec: spec}, "", "", true)
	if e != nil || !v.Container.State.Running {
		t.Fatalf("persisted native intent not recovered: %v", e)
	}
	brokerEnv := strings.Join(v.Container.Config.Env, "\n")
	if !strings.Contains(brokerEnv, "MEDIA_AGENTS_SESSION_EXCHANGE=sessions.custom") || !strings.Contains(brokerEnv, "AGENT_METADATA_EXCHANGE=metadata.custom") {
		t.Fatal("custom Broker exchanges missing")
	}
	proof, e := readRegular(filepath.Join(p.Root, contract.Name(spec.EnvironmentID, spec.WorkspaceID), "identity-token"))
	if e != nil {
		t.Fatal(e)
	}
	identity, stopped, e := svc.Identity(ctx, spec.WorkspaceID, spec.AgentConfigID, "", Generation(v.Container), string(proof), false)
	if e != nil || stopped || identity.Container.ID != "native-id" {
		t.Fatalf("proof rejected: %v", e)
	}
	if _, _, e = svc.Identity(ctx, spec.WorkspaceID, spec.AgentConfigID, "", "", "invalid", false); !errors.Is(e, ErrConflict) {
		t.Fatal("accepted wrong bootstrap proof")
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, e := svc.Ensure(ctx, State{Spec: spec, Version: 1}, "", "", false)
			if e != nil {
				t.Errorf("same create must be idempotent: %v", e)
			}
		})
	}
	wg.Wait()
	if creates != 1 {
		t.Fatalf("created %d containers", creates)
	}
	if v.State.Revision != 1 {
		t.Fatal("same-version retries changed revision")
	}
	spec.Replicas = 0
	if _, e = svc.Ensure(ctx, State{Spec: spec, Version: 2}, "1", "", false); !errors.Is(e, ErrNotImplemented) {
		t.Fatalf("Docker shrink must be unsupported: %v", e)
	}
	lock.Lock()
	container.State.Running = false
	container.State.FinishedAt = "2026-10-13T02:00:00Z"
	lock.Unlock()
	// External native termination is observable without a scaling workflow.
	_, stopped, e = svc.Identity(ctx, spec.WorkspaceID, spec.AgentConfigID, v.Container.ID, Generation(v.Container), "", true)
	if e != nil || !stopped {
		t.Fatal("matching native termination not confirmed")
	}
	if _, e = os.Stat(filepath.Join(p.Root, contract.Name(spec.EnvironmentID, spec.WorkspaceID), "workspace")); e != nil {
		t.Fatal("stop removed persistent workspace")
	}
}

func TestLiveDockerCreation(t *testing.T) {
	if os.Getenv("MEDIA_RUNTIME_DOCKER_TEST") != "1" {
		t.Skip("dedicated live Docker resource test")
	}
	p, spec := testPolicy(t)
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	p.EnvironmentID = "p5-" + suffix
	if len(p.EnvironmentID) > 63 {
		p.EnvironmentID = p.EnvironmentID[:63]
	}
	spec.EnvironmentID = p.EnvironmentID
	spec.CPU = "1"
	name := contract.Name(spec.EnvironmentID, spec.WorkspaceID)
	spec.IdentitySecret = name + "-identity"
	spec.ClaimName = name + "-workspace"
	p.Network = "media-p5-" + suffix
	p.Command = []string{"sh", "-c", "echo healthy >/workspace/health; test -f /workspace/marker || echo retained >/workspace/marker; trap 'exit 0' TERM; while :; do sleep 1; done"}
	p.HealthCommand = []string{"CMD", "sh", "-c", "test -r /workspace/health && test -r /var/run/secrets/media-agent/identity-token"}
	fullHealth := os.Getenv("MEDIA_RUNTIME_DOCKER_REQUIRE_HEALTH") == "1"
	if !fullHealth {
		p.HealthCommand = []string{"NONE"}
	}
	engine := NewEngine("/var/run/docker.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if e := engine.CheckVersion(ctx); e != nil {
		t.Fatal(e)
	}
	var network struct {
		ID string `json:"Id"`
	}
	if e := engine.call(ctx, "POST", "/networks/create", map[string]any{"Name": p.Network, "Labels": map[string]string{"media-agent-test": "p5"}}, &network); e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		_ = engine.call(cleanup, "DELETE", "/networks/"+network.ID, nil, nil)
	}()
	svc := Service{Engine: engine, Policy: p}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		v, e := engine.Inspect(cleanup, name)
		if e == nil && v.Config.Labels[contract.EnvironmentLabel] == spec.EnvironmentID {
			_ = engine.call(cleanup, "DELETE", "/containers/"+v.ID+"?force=true", nil, nil)
		}
	}()
	result, e := svc.Ensure(ctx, State{Spec: spec, Version: 1}, "", "", false)
	if e != nil {
		t.Fatal(e)
	}
	id := result.Container.ID
	generation := Generation(result.Container)
	for fullHealth && !Ready(result.Container) && ctx.Err() == nil {
		time.Sleep(time.Second)
		result, e = svc.Ensure(ctx, State{Spec: spec}, "", "", true)
		if e != nil {
			t.Fatal(e)
		}
	}
	if fullHealth && !Ready(result.Container) {
		t.Fatalf("Docker healthcheck did not become healthy: running=%t state=%+v", result.Container.State.Running, result.Container.State.Health)
	}
	root, workspace, proofPath := svc.paths(spec)
	_ = root
	proof, e := readRegular(proofPath)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = svc.Identity(ctx, spec.WorkspaceID, spec.AgentConfigID, id, generation, string(proof), false); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Ensure(ctx, State{Spec: spec, Version: 1}, "1", "", false); e != nil {
		t.Fatal(e)
	}
	unsupported := spec
	unsupported.Replicas = 0
	if _, e = svc.Ensure(ctx, State{Spec: unsupported, Version: 2}, "1", "", false); !errors.Is(e, ErrNotImplemented) {
		t.Fatalf("Docker shrink must be unsupported: %v", e)
	}
	// Simulate an operator stop through the native Engine, outside the creation API.
	if e = engine.Stop(ctx, id); e != nil {
		t.Fatal(e)
	}
	result, e = svc.Ensure(ctx, State{Spec: spec}, "", "", true)
	if e != nil || result.Container.State.Running {
		t.Fatalf("native stop not observable: %v", e)
	}
	_, terminated, e := svc.Identity(ctx, spec.WorkspaceID, spec.AgentConfigID, id, generation, "", true)
	if e != nil || !terminated {
		t.Fatalf("matching termination missing: generation=%s current=%s state=%+v err=%v", generation, Generation(result.Container), result.Container.State, e)
	}
	spec.Replicas = 1
	result, e = svc.Ensure(ctx, State{Spec: spec, Version: 1}, "", "", false)
	if e != nil {
		t.Fatal(e)
	}
	if result.Container.ID != id || Generation(result.Container) == generation {
		t.Fatal("restart must reuse stable container with new native generation")
	}
	marker, e := os.ReadFile(filepath.Join(workspace, "marker"))
	if e != nil || string(marker) != "retained\n" {
		t.Fatalf("workspace not retained: %v", e)
	}
	if after, e := readRegular(proofPath); e != nil || string(after) != string(proof) {
		t.Fatal("restart changed native bootstrap proof")
	}
	t.Logf("real Engine creation/idempotency/proof/unsupported scaling/external stop/retry/native generation/persistence verified (full Docker health=%t)", fullHealth)
}

func TestCreationProxyBypassesInternalServices(t *testing.T) {
	policy, spec := testPolicy(t)
	policy.HTTPSProxy = "http://proxy.internal:7897"
	policy.NoProxy = "media_agent_gateway,product-broker,172.27.0.1"
	service := Service{Policy: policy}
	body := service.createBody(spec, []byte("proof"), "/workspace", "/proof", "private-bootstrap")
	values := map[string]string{}
	for _, entry := range body["Env"].([]string) {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	if values["HTTPS_PROXY"] != policy.HTTPSProxy || values["https_proxy"] != policy.HTTPSProxy || values["NO_PROXY"] != policy.NoProxy || values["no_proxy"] != policy.NoProxy || values["HTTP_PROXY"] != "" {
		t.Fatal("proxy policy not applied consistently")
	}
	if values["AGENT_GATEWAY_URL"] != spec.GatewayURL || values["AGENT_API_TOKEN"] != "private-bootstrap" {
		t.Fatal("proxy policy replaced runtime identity")
	}
}
