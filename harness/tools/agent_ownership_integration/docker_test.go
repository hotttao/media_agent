package agent_ownership_integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	cfg "media_agent/hertz_gen/config"
	contract "media_agent/hertz_infra/agentworkload"
	"media_agent/hertz_infra/runtimeauth"
	"media_agent/hertz_infra/sessioncredential"
	"media_agent/media_agent_gateway/biz/dal"
	instanceRepo "media_agent/media_agent_gateway/biz/dal/repo/agent_instance"
	sessionRepo "media_agent/media_agent_gateway/biz/dal/repo/agent_session"
	instance "media_agent/media_agent_gateway/biz/domain/agent_instance"
	handler "media_agent/media_agent_gateway/biz/handler/agent_runtime"
	router "media_agent/media_agent_gateway/biz/router/agent_runtime"
	instanceService "media_agent/media_agent_gateway/biz/service/agent_instance"
	sessionService "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
	flow "media_agent/media_agent_gateway/biz/workflow"
	containerHandler "media_agent/media_runtime_controller/biz/handler/container_workload"
	identityHandler "media_agent/media_runtime_controller/biz/handler/runtime_instance_identity"
	"media_agent/media_runtime_controller/biz/middleware"
	containerRouter "media_agent/media_runtime_controller/biz/router/container_workload"
	identityRouter "media_agent/media_runtime_controller/biz/router/runtime_instance_identity"
	native "media_agent/media_runtime_controller/docker"
)

type workloadControlFixtureForDocker struct{ target flow.WorkloadTarget }

func (f *workloadControlFixtureForDocker) Ensure(context.Context, string, string) (flow.WorkloadTarget, error) {
	return f.target, nil
}
func (f *workloadControlFixtureForDocker) List(context.Context, string) ([]flow.WorkloadTarget, string, error) {
	return []flow.WorkloadTarget{f.target}, "", nil
}
func instanceLocatorForDocker(s contract.Spec) instance.ResourceLocator {
	return instance.ResourceLocator{EnvironmentID: s.EnvironmentID, WorkspaceID: s.WorkspaceID, AgentConfigID: s.AgentConfigID, ReplicaID: "0", ResourceName: contract.Name(s.EnvironmentID, s.WorkspaceID)}
}

func dockerCommand(t *testing.T, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if e != nil {
		t.Fatalf("Docker command failed: %v: %s", e, body)
	}
	return body
}

func TestDockerNativeCreationToSessionExecution(t *testing.T) {
	if os.Getenv("MEDIA_RUNTIME_DOCKER_PIPELINE_TEST") != "1" {
		t.Skip("explicit Docker-native pipeline test")
	}
	dsn := os.Getenv("MEDIA_GATEWAY_OWNERSHIP_TEST_DSN")
	broker := os.Getenv("MEDIA_AGENTS_TEST_RABBITMQ_URL")
	network := os.Getenv("MEDIA_RUNTIME_DOCKER_PIPELINE_NETWORK")
	if dsn == "" || broker == "" || network == "" {
		t.Fatal("isolated database, Broker and Docker network required")
	}
	image := os.Getenv("MEDIA_RUNTIME_DOCKER_PIPELINE_IMAGE")
	if image == "" {
		image = "media-agent-pipeline:p8"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	var networks []struct {
		IPAM struct{ Config []struct{ Gateway string } }
	}
	must(t, json.Unmarshal(dockerCommand(t, "network", "inspect", network), &networks))
	if len(networks) != 1 || len(networks[0].IPAM.Config) == 0 {
		t.Fatal("Docker network gateway missing")
	}
	hostGateway := networks[0].IPAM.Config[0].Gateway
	if net.ParseIP(hostGateway) == nil {
		t.Fatal("invalid network gateway")
	}
	config := &cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn, MigrateOnStart: true, MaxOpenConns: 4, MaxIdleConns: 2}
	must(t, dal.Migrate("../../../media_agent_gateway/migrations", config))
	db, e := dal.NewDB(config)
	must(t, e)
	sql, e := db.DB()
	must(t, e)
	defer sql.Close()
	_, private, e := ed25519.GenerateKey(rand.Reader)
	must(t, e)
	signer, e := runtimeauth.New(private)
	must(t, e)
	tokenKey := make([]byte, 32)
	_, e = rand.Read(tokenKey)
	must(t, e)
	keys, e := sessioncredential.New("fixture", map[string][]byte{"fixture": tokenKey})
	must(t, e)
	root := t.TempDir()
	publicFile := filepath.Join(root, "keys.json")
	encoded, e := json.Marshal(map[string]any{"keys": signer.PublicKeys()})
	must(t, e)
	must(t, os.WriteFile(publicFile, encoded, 0600))
	gatewayPort := port(t)
	spec := contract.Spec{Driver: "docker", EnvironmentID: "environment", WorkspaceID: "workspace", AgentConfigID: "config", AgentType: "codex", Image: image, Replicas: 1, Port: 3001, CPU: "500m", Memory: "256Mi", StorageSize: "1Gi", GatewayURL: fmt.Sprintf("http://%s:%d", hostGateway, gatewayPort), SharedSecret: "fixture", SessionExchange: "test.docker.pipeline.sessions"}
	name := contract.Name(spec.EnvironmentID, spec.WorkspaceID)
	spec.IdentitySecret = name + "-identity"
	spec.ClaimName = name + "-workspace"
	if e := exec.Command("docker", "inspect", name).Run(); e == nil {
		t.Fatal("fixture container already exists; refusing to reuse it")
	}
	// Cleanup also requires this run's private bootstrap hash, never labels alone.
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := exec.Command("docker", "logs", "--tail", "60", name).CombinedOutput()
			t.Logf("owned Agent container logs: %s", logs)
		}
		inspect := exec.Command("docker", "inspect", name)
		body, e := inspect.Output()
		if e != nil {
			return
		}
		var rows []struct {
			ID     string
			Config struct{ Labels map[string]string }
		}
		proof, e := os.ReadFile(filepath.Join(root, "runtime", name, "identity-token"))
		if e != nil {
			return
		}
		hash := sha256.Sum256(proof)
		if json.Unmarshal(body, &rows) == nil && len(rows) == 1 && rows[0].Config.Labels[contract.Group+"/bootstrap-hash"] == hex.EncodeToString(hash[:]) && rows[0].Config.Labels[contract.EnvironmentLabel] == spec.EnvironmentID && rows[0].Config.Labels[contract.WorkspaceLabel] == spec.WorkspaceID {
			_ = exec.Command("docker", "rm", "-f", rows[0].ID).Run()
		}
	})
	policy := native.Policy{EnvironmentID: spec.EnvironmentID, Network: network, Root: filepath.Join(root, "runtime"), PublicKeysFile: publicFile, DatabaseURL: "postgresql+asyncpg://postgres:p8fixture@postgres:5432/agent_runtime", RabbitURL: "amqp://guest:guest@rabbitmq:5672/", SharedSecret: spec.SharedSecret, Images: map[string]string{"codex": image}, AgentTokens: map[string]string{"config": "agent-test-token"}}
	engine := native.NewEngine("/var/run/docker.sock")
	must(t, engine.CheckVersion(ctx))
	service := &native.Service{Engine: engine, Policy: policy}
	creationToken := "fixture-creation"
	middleware.Configure(creationToken)
	containerHandler.Configure(service)
	identityHandler.Configure(service)
	defer containerHandler.Configure(nil)
	defer identityHandler.Configure(nil)
	creationAddress := fmt.Sprintf("127.0.0.1:%d", port(t))
	creationH := server.New(server.WithHostPorts(creationAddress))
	containerRouter.Register(creationH)
	identityRouter.Register(creationH)
	creation := mount(t, creationH, creationAddress)
	defer creation.Close()
	resources := client.DockerWorkloads{EnvironmentID: spec.EnvironmentID, BaseURL: creation.URL, ServiceToken: creationToken}
	authority := client.DockerIdentity{EnvironmentID: spec.EnvironmentID, BaseURL: creation.URL, ServiceToken: creationToken}
	instances := instanceService.New(instanceRepo.NewRegistration(db), dal.NewTxRunner(db))
	registration := flow.NewInstanceRegistration(credentialAuthority{}, credentialAuthority{}, placement{}, authority, instances, keys)
	store := sessionRepo.New(db)
	sessions := sessionService.New(store, fixedContext{}).WithCredentials(client.SessionCredentials{Keys: keys})
	// This Gateway fixture runs on the host; only its owned Docker DNS name is
	// translated to a native container IP. Production peers share Docker DNS.
	transport := &http.Transport{DialContext: func(call context.Context, kind, address string) (net.Conn, error) {
		host, p, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		if host != name || p != "3001" {
			return nil, fmt.Errorf("unexpected fixture destination")
		}
		body, e := exec.CommandContext(call, "docker", "inspect", name).Output()
		if e != nil {
			return nil, e
		}
		var rows []struct {
			NetworkSettings struct {
				Networks map[string]struct{ IPAddress string }
			}
		}
		if json.Unmarshal(body, &rows) != nil || len(rows) != 1 {
			return nil, fmt.Errorf("invalid container inspection")
		}
		ip := rows[0].NetworkSettings.Networks[network].IPAddress
		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("container IP missing")
		}
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(call, kind, net.JoinHostPort(ip, p))
	}}
	defer transport.CloseIdleConnections()
	runtimeHTTP := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	factory := client.ScopedRuntimeFactory{Signer: signer, HTTP: runtimeHTTP}
	routing := flow.NewSessionRouting(store, sessions, instances, placement{}, authority, factory)
	execution := flow.NewExecutionContext(registration, store, sessions, routing)
	handler.Configure(registration, execution)
	control := &workloadControlFixtureForDocker{target: flow.WorkloadTarget{Spec: spec, Version: 1, OwnerUserID: "user"}}
	provision := &flow.WorkloadProvisioning{Control: control, Resources: resources, EnvironmentID: spec.EnvironmentID, Instances: instances, Bindings: store, Routing: routing, Factory: factory, Native: authority}
	source, e := sessionevents.NewRabbitSource(broker, spec.SessionExchange, 256)
	must(t, e)
	defer source.Close()
	run := flow.NewRunTurn(sessions, nil).WithRouting(routing)
	gatewayAddress := fmt.Sprintf("0.0.0.0:%d", gatewayPort)
	h := server.New(server.WithHostPorts(gatewayAddress))
	router.Register(h)
	mountEvents(h, flow.NewEventStream(run, source))
	gateway := mount(t, h, gatewayAddress)
	defer gateway.Close()
	gateway.URL = fmt.Sprintf("http://127.0.0.1:%d", gatewayPort)
	defer db.Exec("DELETE FROM agent_instance_history WHERE workspace_id='workspace' AND agent_config_id='config'")
	defer db.Exec("DELETE FROM agent_instances WHERE workspace_id='workspace' AND agent_config_id='config'")
	must(t, provision.Ensure(ctx, "user", "workspace"))
	wait(t, ctx, func() bool { v, e := resources.Read(ctx, spec); return e == nil && v.ReadyReplicas == 1 })
	old, e := instances.Get(ctx, "environment", "workspace", "config", "0")
	must(t, e)
	if old.ResourceUID == "" || old.ResourceGeneration == "" {
		t.Fatal("native identity not registered")
	}
	if _, e = authority.Resolve(ctx, instanceLocatorForDocker(spec), "wrong-proof"); e == nil {
		t.Fatal("invalid native proof accepted")
	}
	repeated, e := resources.Read(ctx, spec)
	must(t, e)
	must(t, provision.Ensure(ctx, "user", "workspace"))
	after, e := resources.Read(ctx, spec)
	must(t, e)
	if after.Revision != repeated.Revision {
		t.Fatal("idempotent creation changed revision")
	}
	session, e := sessions.Create(ctx, "user", "workspace", "Docker pipeline", "")
	must(t, e)
	defer db.Exec("DELETE FROM media_agent_gateway.agent_sessions WHERE id=?", session.ID)
	_, e = run.Register(ctx, session, "")
	must(t, e)
	streams, stop := context.WithCancel(ctx)
	defer stop()
	firstStream, firstReader := openEvents(t, streams, gateway.URL, session.ID, 0)
	defer firstStream.Body.Close()
	secondStream, secondReader := openEvents(t, streams, gateway.URL, session.ID, 0)
	defer secondStream.Body.Close()
	first, e := run.Start(ctx, session.ID, "user", "browser-cookie", "first", "hold")
	must(t, e)
	duplicate, e := run.Start(ctx, session.ID, "user", "browser-cookie", "first", "hold")
	must(t, e)
	if duplicate.TurnID != first.TurnID {
		t.Fatal("duplicate created a second Turn")
	}
	queued, e := run.Start(ctx, session.ID, "user", "browser-cookie", "second", "queued")
	must(t, e)
	if queued.Status != "queued" {
		t.Fatal("single Session violated FIFO")
	}
	awaitSSE(t, firstReader, "item.delta")
	awaitSSE(t, secondReader, "item.delta")
	workspace := filepath.Join(policy.Root, name, "workspace")
	must(t, os.WriteFile(filepath.Join(workspace, "release"), []byte("release"), 0600))
	peer, e := routing.Resolve(ctx, session)
	must(t, e)
	wait(t, ctx, func() bool {
		v, e := peer.GetTurn(ctx, session.ID, queued.TurnID)
		return e == nil && v.Status == "completed"
	})
	firstStream.Body.Close()
	secondStream.Body.Close()
	replay, replayReader := openEvents(t, streams, gateway.URL, session.ID, 0)
	awaitSSE(t, replayReader, "item.completed")
	replay.Body.Close()
	events, e := run.EventsAfter(ctx, session.ID, "user", 0)
	must(t, e)
	if len(events) < 2 {
		t.Fatal("durable replay missing")
	}
	for i, event := range events {
		if event.Sequence != int64(i+1) {
			t.Fatal("durable sequence gap or persisted delta")
		}
	}
	must(t, os.WriteFile(filepath.Join(workspace, "marker"), []byte("retained"), 0600))
	dockerCommand(t, "restart", "--time", "5", name)
	wait(t, ctx, func() bool { v, e := resources.Read(ctx, spec); return e == nil && v.ReadyReplicas == 1 })
	current, e := instances.Get(ctx, "environment", "workspace", "config", "0")
	must(t, e)
	if current.ResourceUID != old.ResourceUID || current.ResourceGeneration == old.ResourceGeneration || current.IncarnationID == old.IncarnationID {
		t.Fatal("restart native identity mismatch")
	}
	peer, e = routing.Resolve(ctx, session)
	must(t, e)
	facts, e := peer.GetRuntimeSession(ctx, session.ID)
	must(t, e)
	if facts.ContextVersion != 7 || facts.RuntimeSessionID != "native-"+session.ID || facts.ExecutionEpoch != 2 {
		t.Fatalf("restart lost fixed context or ownership: %+v", facts)
	}
	marker, e := os.ReadFile(filepath.Join(workspace, "marker"))
	must(t, e)
	if string(marker) != "retained" {
		t.Fatal("restart lost workspace")
	}
	stale := factory.ForInstance(*old, 1)
	_, e = stale.SubmitTurn(ctx, client.SubmitTurnInput{SessionID: session.ID, IdempotencyKey: "old-owner", ContextVersion: 7, Input: map[string]any{"message": "forbidden"}})
	var apiError *client.APIError
	if !errors.As(e, &apiError) || apiError.StatusCode < 400 || apiError.StatusCode >= 500 {
		t.Fatalf("old incarnation write not rejected: %v", e)
	}
	if e = provision.Scale(ctx, "user", "workspace", "scale", 2, 1); !errors.Is(e, flow.ErrWorkloadNotImplemented) {
		t.Fatalf("Docker scale: %v", e)
	}
	// Stop SSE polling before removing the Session or closing its database.
	gateway.Close()
	t.Logf("real Docker creation/identity/Agent registration/FIFO/SSE/replay/restart/workspace verified; epochs=%d, events=%s", facts.ExecutionEpoch, strconv.Itoa(len(events)))
}
