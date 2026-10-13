package agent_ownership_integration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	"github.com/google/uuid"
	"io"
	cfg "media_agent/hertz_gen/config"
	contract "media_agent/hertz_infra/agentworkload"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/runtimeauth"
	"media_agent/hertz_infra/sessioncredential"
	"media_agent/media_agent_gateway/biz/dal"
	instanceRepo "media_agent/media_agent_gateway/biz/dal/repo/agent_instance"
	sessionRepo "media_agent/media_agent_gateway/biz/dal/repo/agent_session"
	instance "media_agent/media_agent_gateway/biz/domain/agent_instance"
	sessionDomain "media_agent/media_agent_gateway/biz/domain/agent_session"
	handler "media_agent/media_agent_gateway/biz/handler/agent_runtime"
	router "media_agent/media_agent_gateway/biz/router/agent_runtime"
	instanceService "media_agent/media_agent_gateway/biz/service/agent_instance"
	sessionService "media_agent/media_agent_gateway/biz/service/agent_session"
	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/shared/sessionevents"
	flow "media_agent/media_agent_gateway/biz/workflow"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type credentialAuthority struct{}

func (credentialAuthority) VerifyAgentCredential(_ context.Context, token string) (internaljwt.AgentPrincipal, error) {
	if token != "agent-test-token" {
		return internaljwt.AgentPrincipal{}, client.ErrAgentCredentialRejected
	}
	return internaljwt.AgentPrincipal{UserID: "user", ActorID: "Agent:config", OrganizationID: "org", CredentialID: "credential"}, nil
}
func (credentialAuthority) ResolveAgentIdentity(context.Context, string) (client.AgentIdentity, error) {
	return client.AgentIdentity{ConfigurationID: "config", ActorID: "Agent:config", WorkspaceID: "workspace", OwnerUserID: "user", OrganizationID: "org", State: "running", OwnerAuthorized: true}, nil
}

type placement struct{}

func (placement) Environment(context.Context, string, string) (string, error) {
	return "environment", nil
}

type fixedContext struct{}

func (fixedContext) Default(context.Context, string) (sessionService.AgentContext, error) {
	return sessionService.AgentContext{WorkspaceID: "workspace", AgentConfigID: "config", AgentID: "codex", AgentType: "codex", APIKey: "fixed-version-provider-key", ContextVersion: 7}, nil
}
func (c fixedContext) Resolve(context.Context, string, string, string) (sessionService.AgentContext, error) {
	return c.Default(context.Background(), "")
}
func (c fixedContext) ResolveVersion(_ context.Context, _, _, _ string, version int64) (sessionService.AgentContext, error) {
	v, e := c.Default(context.Background(), "")
	if version != 7 {
		return v, errors.New("wrong pinned version")
	}
	return v, e
}

type nativeFixture struct {
	mu   sync.Mutex
	rows map[string]instance.ResourceFacts
}

func (n *nativeFixture) Resolve(_ context.Context, l instance.ResourceLocator, proof string) (instance.ResourceFacts, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	v, ok := n.rows[l.ReplicaID]
	if !ok || proof != "isolated-native-proof" {
		return v, client.ErrNativeIdentity
	}
	return v, nil
}
func (n *nativeFixture) Observe(_ context.Context, v instance.Instance) (instance.ResourceFacts, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	f, ok := n.rows[v.ReplicaID]
	if !ok {
		return f, client.ErrNativeUnavailable
	}
	return f, nil
}
func (n *nativeFixture) Terminated(context.Context, instance.Instance) (bool, error) {
	return false, nil
}
func (n *nativeFixture) notReady(replica string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	v := n.rows[replica]
	v.Ready = false
	n.rows[replica] = v
}

func (n *nativeFixture) ready(replica string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	v := n.rows[replica]
	v.Ready = true
	n.rows[replica] = v
}

type lostResponseFactory struct {
	client.ScopedRuntimeFactory
	lost atomic.Bool
}

func (f *lostResponseFactory) ForInstance(v instance.Instance, epoch int64) client.ControlledRuntime {
	return lostResponseRuntime{f.ScopedRuntimeFactory.ForInstance(v, epoch), f}
}

type lostResponseRuntime struct {
	client.ControlledRuntime
	factory *lostResponseFactory
}

func (r lostResponseRuntime) Transfer(ctx context.Context, id string, input client.TransferSessionInput) (*client.RuntimeSession, error) {
	v, e := r.ControlledRuntime.Transfer(ctx, id, input)
	if e == nil && !r.factory.lost.Swap(true) {
		return nil, errors.New("isolated lost transfer response")
	}
	return v, e
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func port(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	must(t, e)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type gatewayEndpoint struct {
	URL string
	h   *server.Hertz
}

func (g *gatewayEndpoint) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = g.h.Shutdown(ctx)
}
func mount(t *testing.T, h *server.Hertz, address string) *gatewayEndpoint {
	t.Helper()
	h.GET("/test/ready", func(_ context.Context, c *app.RequestContext) { c.Status(200) })
	go func() { _ = h.Run() }()
	gateway := &gatewayEndpoint{URL: "http://" + address, h: h}
	readyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	wait(t, readyCtx, func() bool {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	})
	return gateway
}

// Owner authorization is a fixture; HTTP transport, stream reconciliation,
// independent Rabbit queues and the execution workflows are production code.
func mountEvents(h *server.Hertz, stream *flow.EventStream) {
	h.GET("/test/sessions/:session/events", func(ctx context.Context, c *app.RequestContext) {
		after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
		writer := sse.NewWriter(c)
		if err := writer.WriteEvent("", "ready", []byte(`{}`)); err != nil {
			return
		}
		_ = stream.Stream(ctx, c.Param("session"), "user", after, func(event flow.StreamEvent) error {
			if event.Heartbeat {
				return nil
			}
			id := ""
			if event.Sequence > 0 {
				id = strconv.FormatInt(event.Sequence, 10)
			}
			return writer.WriteEvent(id, "message", event.Payload)
		})
	})
}
func openEvents(t *testing.T, ctx context.Context, gateway, session string, after int64) (*http.Response, *bufio.Reader) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/test/sessions/%s/events?after=%d", gateway, session, after), nil)
	must(t, err)
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	if resp.StatusCode != 200 {
		t.Fatalf("SSE status: %d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	must(t, err)
	if !strings.Contains(line, "ready") {
		t.Fatalf("missing SSE ready: %s", line)
	}
	return resp, reader
}
func awaitSSE(t *testing.T, reader *bufio.Reader, contains string) {
	t.Helper()
	for {
		line, err := reader.ReadString('\n')
		must(t, err)
		if strings.Contains(line, contains) {
			return
		}
	}
}

func TestSignedRegistrationQueuedCredentialsAndLostTransferRecovery(t *testing.T) {
	dsn := os.Getenv("MEDIA_GATEWAY_OWNERSHIP_TEST_DSN")
	pythonDB := os.Getenv("MEDIA_AGENTS_TEST_DB_URL")
	broker := os.Getenv("MEDIA_AGENTS_TEST_RABBITMQ_URL")
	if dsn == "" || pythonDB == "" || broker == "" {
		t.Skip("isolated PostgreSQL and RabbitMQ required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	config := &cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn, MigrateOnStart: true}
	must(t, dal.Migrate("../../../media_agent_gateway/migrations", config))
	db, e := dal.NewDB(config)
	must(t, e)
	sqlDB, e := db.DB()
	must(t, e)
	defer sqlDB.Close()
	secondDB, e := dal.NewDB(config)
	must(t, e)
	secondSQL, e := secondDB.DB()
	must(t, e)
	defer secondSQL.Close()
	_, private, e := ed25519.GenerateKey(rand.Reader)
	must(t, e)
	signer, e := runtimeauth.New(private)
	must(t, e)
	keys, e := sessioncredential.New("test", map[string][]byte{"test": bytes.Repeat([]byte{9}, 32)})
	must(t, e)
	root := t.TempDir()
	publicFile := filepath.Join(root, "keys.json")
	raw, e := json.Marshal(map[string]any{"keys": signer.PublicKeys()})
	must(t, e)
	must(t, os.WriteFile(publicFile, raw, 0600))
	proofFile := filepath.Join(root, "proof")
	must(t, os.WriteFile(proofFile, []byte("isolated-native-proof"), 0600))
	native := &nativeFixture{rows: map[string]instance.ResourceFacts{}}
	ports := []int{port(t), port(t)}
	for i, p := range ports {
		ordinal := strconv.Itoa(i)
		native.rows[ordinal] = instance.ResourceFacts{Locator: instance.ResourceLocator{EnvironmentID: "environment", WorkspaceID: "workspace", AgentConfigID: "config", ReplicaID: ordinal, Namespace: "test", ResourceName: "agents-" + ordinal, ResourceUID: "native-" + ordinal}, Generation: "generation-" + ordinal, AgentType: "codex", Endpoint: fmt.Sprintf("http://127.0.0.1:%d", p), Eligible: true, Ready: true, Running: true}
	}
	instances := instanceService.New(instanceRepo.NewRegistration(db), dal.NewTxRunner(db))
	registration := flow.NewInstanceRegistration(credentialAuthority{}, credentialAuthority{}, placement{}, native, instances, keys)
	store := sessionRepo.New(db)
	sessions := sessionService.New(store, fixedContext{}).WithCredentials(client.SessionCredentials{Keys: keys})
	factory := &lostResponseFactory{ScopedRuntimeFactory: client.ScopedRuntimeFactory{Signer: signer}}
	routing := flow.NewSessionRouting(store, sessions, instances, placement{}, native, factory)
	secondStore := sessionRepo.New(secondDB)
	secondSessions := sessionService.New(secondStore, fixedContext{}).WithCredentials(client.SessionCredentials{Keys: keys})
	secondRouting := flow.NewSessionRouting(secondStore, secondSessions, instances, placement{}, native, factory)
	execution := flow.NewExecutionContext(registration, store, sessions, routing)
	handler.Configure(registration, execution)
	mcpIdentity := flow.NewMCPIdentity(sessions, credentialAuthority{}, credentialAuthority{}, execution)
	source, e := sessionevents.NewRabbitSource(broker, "test.ownership.sessions", 256)
	must(t, e)
	defer source.Close()
	secondSource, e := sessionevents.NewRabbitSource(broker, "test.ownership.sessions", 256)
	must(t, e)
	defer secondSource.Close()
	run := flow.NewRunTurn(sessions, nil).WithRouting(routing)
	secondRun := flow.NewRunTurn(secondSessions, nil).WithRouting(secondRouting)
	address := fmt.Sprintf("127.0.0.1:%d", port(t))
	h := server.New(server.WithHostPorts(address))
	router.Register(h)
	mountEvents(h, flow.NewEventStream(run, source))
	gateway := mount(t, h, address)
	defer gateway.Close()
	secondAddress := fmt.Sprintf("127.0.0.1:%d", port(t))
	secondH := server.New(server.WithHostPorts(secondAddress))
	router.Register(secondH)
	mountEvents(secondH, flow.NewEventStream(secondRun, secondSource))
	secondGateway := mount(t, secondH, secondAddress)
	defer secondGateway.Close()
	release := filepath.Join(root, "release")
	command := exec.CommandContext(ctx, "../../../media_agents/.venv/bin/python", "../../../media_agents/scripts/ownership_pipeline_server.py")
	command.Env = append(os.Environ(), "TEST_GATEWAY_URL="+gateway.URL, "TEST_GATEWAY_URL_1="+secondGateway.URL, "TEST_RUNTIME_BOOT_ID="+uuid.NewString(), "TEST_RUNTIME_ROOT="+root, "TEST_RUNTIME_PUBLIC_KEYS="+publicFile, "TEST_RUNTIME_PROOF="+proofFile, "TEST_RUNTIME_RELEASE="+release, "TEST_RUNTIME_PORT_0="+strconv.Itoa(ports[0]), "TEST_RUNTIME_PORT_1="+strconv.Itoa(ports[1]))
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	must(t, command.Start())
	defer func() { _ = command.Process.Kill(); _ = command.Wait(); t.Log(output.String()) }()
	defer db.Exec("DELETE FROM agent_instance_history WHERE workspace_id='workspace' AND agent_config_id='config'")
	defer db.Exec("DELETE FROM agent_instances WHERE workspace_id='workspace' AND agent_config_id='config'")
	httpClient := &http.Client{Timeout: time.Second}
	for _, p := range ports {
		wait(t, ctx, func() bool {
			resp, e := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/health", p))
			if e != nil {
				return false
			}
			defer resp.Body.Close()
			return resp.StatusCode == 200
		})
	}
	session, e := sessions.Create(ctx, "user", "workspace", "Ownership", "")
	must(t, e)
	defer db.Exec("DELETE FROM media_agent_gateway.agent_sessions WHERE id=?", session.ID)
	subscription, e := source.Open(ctx, session.ID)
	must(t, e)
	defer subscription.Close()
	_, e = run.Register(ctx, session, "")
	must(t, e)
	streamCtx, stopStreams := context.WithCancel(ctx)
	defer stopStreams()
	firstStream, firstReader := openEvents(t, streamCtx, gateway.URL, session.ID, 0)
	defer firstStream.Body.Close()
	secondStream, secondReader := openEvents(t, streamCtx, secondGateway.URL, session.ID, 0)
	defer secondStream.Body.Close()
	first, e := run.Start(ctx, session.ID, "user", "browser-cookie", "first", "hold")
	must(t, e)
	queued, e := run.Start(ctx, session.ID, "user", "browser-cookie", "second", "queued")
	must(t, e)
	awaitSSE(t, firstReader, "item.delta")
	awaitSSE(t, secondReader, "item.delta")
	if queued.Status != "queued" {
		t.Fatal("queued Turn executed in parallel")
	}
	binding, e := store.GetSession(ctx, session.ID)
	must(t, e)
	old := *binding
	wait(t, ctx, func() bool {
		select {
		case event := <-subscription.Events:
			return event.Sequence == 0 && routing.AllowLive(ctx, session.ID, event.Payload)
		default:
			return false
		}
	})
	instanceRecord, e := instances.Get(ctx, "environment", "workspace", "config", binding.ReplicaID)
	must(t, e)
	instanceCredential, e := keys.Derive("it1", instanceRecord.CredentialKeyID, instanceRecord.EnvironmentID, instanceRecord.WorkspaceID, instanceRecord.AgentConfigID, instanceRecord.ReplicaID, instanceRecord.IncarnationID, instanceRecord.ResourceUID, instanceRecord.ResourceGeneration)
	must(t, e)
	sessionToken, e := sessions.EnsureToken(ctx, binding, true)
	must(t, e)
	mcpCredentials := flow.MCPCredentials{AgentToken: "agent-test-token", SessionToken: sessionToken, InstanceCredential: instanceCredential, ReplicaID: binding.ReplicaID, IncarnationID: binding.IncarnationID, TurnID: first.TurnID, ExecutionEpoch: binding.ExecutionEpoch}
	verified, e := mcpIdentity.Authorize(ctx, mcpCredentials)
	must(t, e)
	if verified.SessionID != session.ID || verified.ContextVersion != 7 || verified.UserID != "user" {
		t.Fatal("MCP fixed scope mismatch", verified)
	}
	for _, change := range []func(*flow.MCPCredentials){func(c *flow.MCPCredentials) { c.SessionToken = "browser-cookie" }, func(c *flow.MCPCredentials) { c.AgentToken = "other-agent" }, func(c *flow.MCPCredentials) { c.IncarnationID = "other-boot" }, func(c *flow.MCPCredentials) { c.ExecutionEpoch++ }, func(c *flow.MCPCredentials) { c.TurnID = queued.TurnID }} {
		invalid := mcpCredentials
		change(&invalid)
		if _, e = mcpIdentity.Authorize(ctx, invalid); e == nil {
			t.Fatal("MCP accepted wrong execution identity")
		}
	}

	native.notReady(binding.ReplicaID)
	if _, e = routing.Resolve(ctx, binding); !errors.Is(e, flow.ErrRuntimeWaiting) {
		t.Fatalf("running source transferred without stop evidence: %v", e)
	}
	intent, e := store.GetSession(ctx, session.ID)
	must(t, e)
	if intent.BindingPhase != "transferring" {
		t.Fatal("intent was not persisted")
	}
	must(t, os.WriteFile(release, []byte("finish"), 0600))
	wait(t, ctx, func() bool {
		peer, e := routing.Facts(ctx, binding)
		if e != nil {
			return false
		}
		firstStream.Body.Close()
		secondStream.Body.Close()
		replay, replayReader := openEvents(t, streamCtx, secondGateway.URL, session.ID, 0)
		awaitSSE(t, replayReader, "item.completed")
		replay.Body.Close()
		facts, e := peer.GetRuntimeSession(ctx, session.ID)
		return e == nil && facts.ActiveTurnID == ""
	})
	if _, e = routing.Resolve(ctx, binding); !errors.Is(e, flow.ErrRuntimeWaiting) {
		t.Fatalf("expected lost transfer response: %v", e)
	}
	peer, e := secondRouting.Resolve(ctx, binding)
	must(t, e)
	wait(t, ctx, func() bool {
		turn, e := peer.GetTurn(ctx, session.ID, queued.TurnID)
		return e == nil && turn.Status == "completed"
	})
	facts, e := peer.GetRuntimeSession(ctx, session.ID)
	must(t, e)
	if facts.ExecutionEpoch != 2 || facts.ContextVersion != 7 || facts.RuntimeSessionID != "native-"+session.ID || facts.LastTransferID != intent.TransferID {
		t.Fatalf("lost fixed facts: %+v", facts)
	}
	stalePeer := client.ScopedRuntimeFactory{Signer: signer}.ForInstance(instance.Instance{WorkspaceID: "workspace", AgentConfigID: "config", ReplicaID: old.ReplicaID, IncarnationID: old.IncarnationID, Endpoint: native.rows[old.ReplicaID].Endpoint}, 1)
	_, e = stalePeer.SubmitTurn(ctx, client.SubmitTurnInput{SessionID: session.ID, IdempotencyKey: "stale", ContextVersion: 7, Input: map[string]any{"message": "stale"}})
	var apiErr *client.APIError
	if !errors.As(e, &apiErr) || apiErr.StatusCode != 409 {
		t.Fatalf("old owner accepted write: %v", e)
	}
	if _, e = mcpIdentity.Authorize(ctx, mcpCredentials); e == nil {
		t.Fatal("old MCP instance retained permission after transfer")
	}
	oldDelta, _ := json.Marshal(map[string]any{"type": "item.delta", "owner_instance_id": old.ReplicaID, "owner_incarnation_id": old.IncarnationID, "execution_epoch": 1})
	if secondRouting.AllowLive(ctx, session.ID, oldDelta) {
		t.Fatal("stale delta accepted")
	}
	firstTurn, e := peer.GetTurn(ctx, session.ID, first.TurnID)
	must(t, e)
	if firstTurn.Status != "completed" {
		t.Fatal("completed source Turn was replayed")
	}
	events, e := peer.ReplayEvents(ctx, session.ID, 0, 1000)
	must(t, e)
	if len(events) == 0 {
		t.Fatal("no durable recovery facts")
	}
	for _, event := range events {
		if event.Sequence < 1 || bytes.Contains(event.Payload, []byte("provider-key")) || bytes.Contains(event.Payload, []byte("browser-cookie")) {
			t.Fatal("event leaked credentials or persisted live delta")
		}
	}
	// Recover an assigned Session whose Agent row was never created, after
	// its original transfer target incarnation has become unavailable.
	native.ready(old.ReplicaID)
	missing, e := sessions.Create(ctx, "user", "workspace", "Initial registration gap", "")
	must(t, e)
	defer db.Exec("DELETE FROM media_agent_gateway.agent_sessions WHERE id=?", missing.ID)
	_, e = routing.Resolve(ctx, missing)
	must(t, e)
	initial := *missing
	alternative := "0"
	if initial.ReplicaID == "0" {
		alternative = "1"
	}
	_, e = store.BeginTransfer(ctx, missing.ID, missing.BindingVersion, sessionDomain.Binding{EnvironmentID: "environment", ReplicaID: alternative, IncarnationID: "expired-target", ExecutionEpoch: 1}, "initial-gap-transfer")
	must(t, e)
	replacementPeer, e := secondRouting.Resolve(ctx, missing)
	must(t, e)
	recovered, e := replacementPeer.GetRuntimeSession(ctx, missing.ID)
	must(t, e)
	if recovered.ExecutionEpoch != 2 || recovered.LastTransferID != "initial-gap-transfer" || recovered.OwnerInstanceID != alternative {
		t.Fatalf("missing registration/target recovery: %+v", recovered)
	}
	if _, e = run.Register(ctx, missing, ""); e != nil {
		t.Fatal(e)
	}
	lateSource := client.ScopedRuntimeFactory{Signer: signer}.ForInstance(instance.Instance{WorkspaceID: "workspace", AgentConfigID: "config", ReplicaID: initial.ReplicaID, IncarnationID: initial.IncarnationID, Endpoint: native.rows[initial.ReplicaID].Endpoint}, 1)
	_, e = lateSource.RegisterRuntimeSession(ctx, client.RegisterRuntimeSessionInput{SessionID: missing.ID, WorkspaceID: "workspace", AgentConfigID: "config", ContextVersion: 7, ContextRef: client.SessionContextReference(missing)})
	if !errors.As(e, &apiErr) || apiErr.StatusCode != 409 {
		t.Fatalf("late initial registration replaced new owner: %v", e)
	}
	gapTurn, e := run.Start(ctx, missing.ID, "user", "browser-cookie", "gap-turn", "quick")
	must(t, e)
	wait(t, ctx, func() bool {
		turn, e := replacementPeer.GetTurn(ctx, missing.ID, gapTurn.TurnID)
		return e == nil && turn.Status == "completed"
	})
	unsigned, e := http.NewRequestWithContext(ctx, "GET", native.rows[old.ReplicaID].Endpoint+"/api/v1/runtime-sessions/"+session.ID, nil)
	must(t, e)
	resp, e := httpClient.Do(unsigned)
	must(t, e)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("unsigned managed request accepted")
	}

	t.Run("P5 two Gateways drain running and transfer queued before native shrink", func(t *testing.T) {
		native.ready("0")
		native.ready("1")
		target := &workloadAuthority{value: client.WorkloadTarget{Spec: contract.Spec{Driver: "kubernetes", Namespace: "test", EnvironmentID: "environment", WorkspaceID: "workspace", AgentConfigID: "config", AgentType: "codex", Replicas: 2}, Version: 1, OwnerUserID: "user"}}
		resources := &workloadResources{native: native}
		firstGateway := &flow.WorkloadProvisioning{Control: target, Resources: resources, EnvironmentID: "environment", Instances: instances, Bindings: store, Routing: routing, Factory: factory, Native: native}
		secondGateway := &flow.WorkloadProvisioning{Control: target, Resources: resources, EnvironmentID: "environment", Instances: instances, Bindings: secondStore, Routing: secondRouting, Factory: factory, Native: native}
		must(t, firstGateway.Ensure(ctx, "user", "workspace"))
		scaleSession, e := sessions.Create(ctx, "user", "workspace", "Scale running and queued", "")
		must(t, e)
		defer db.Exec("DELETE FROM media_agent_gateway.agent_sessions WHERE id=?", scaleSession.ID)
		member, e := instances.Get(ctx, "environment", "workspace", "config", "1")
		must(t, e)
		scaleSession, e = store.AssignBinding(ctx, scaleSession.ID, scaleSession.BindingVersion, sessionDomain.Binding{EnvironmentID: "environment", ReplicaID: "1", IncarnationID: member.IncarnationID, ExecutionEpoch: 1})
		must(t, e)
		_, e = run.Register(ctx, scaleSession, "")
		must(t, e)
		must(t, os.Remove(release))
		held, e := run.Start(ctx, scaleSession.ID, "user", "browser-cookie", "scale-hold", "hold")
		must(t, e)
		queuedScale, e := run.Start(ctx, scaleSession.ID, "user", "browser-cookie", "scale-queued", "queued")
		must(t, e)
		if queuedScale.Status != "queued" {
			t.Fatal("scale queue bypassed running turn")
		}
		target.value.Version = 2
		target.value.Spec.Replicas = 1
		if e = firstGateway.Ensure(ctx, "user", "workspace"); !errors.Is(e, flow.ErrRuntimeWaiting) {
			t.Fatalf("scale intent: %v", e)
		}
		if e = secondGateway.Ensure(ctx, "user", "workspace"); !errors.Is(e, flow.ErrRuntimeWaiting) {
			t.Fatalf("running drain: %v", e)
		}
		state, e := resources.Read(ctx, target.value.Spec)
		must(t, e)
		if state.Spec.Replicas != 2 {
			t.Fatal("native count lowered before held execution finished")
		}
		must(t, os.WriteFile(release, []byte("finish"), 0600))
		wait(t, ctx, func() bool {
			e := secondGateway.Ensure(ctx, "user", "workspace")
			if e != nil && !errors.Is(e, flow.ErrRuntimeWaiting) && !errors.Is(e, flow.ErrWorkloadConflict) {
				t.Fatalf("scale recovery: %v", e)
			}
			state, e := resources.Read(ctx, target.value.Spec)
			return e == nil && state.Spec.Replicas == 1 && state.Operation == ""
		})
		bound, e := store.GetSession(ctx, scaleSession.ID)
		must(t, e)
		if bound.ReplicaID != "0" || bound.ExecutionEpoch != 2 {
			t.Fatalf("queued ownership not transferred: %+v", bound)
		}
		targetPeer, e := secondRouting.Facts(ctx, bound)
		must(t, e)
		wait(t, ctx, func() bool {
			turn, e := targetPeer.GetTurn(ctx, scaleSession.ID, queuedScale.TurnID)
			return e == nil && turn.Status == "completed"
		})
		completed, e := targetPeer.GetTurn(ctx, scaleSession.ID, held.TurnID)
		must(t, e)
		if completed.Status != "completed" {
			t.Fatal("source completed turn changed")
		}
		retired, e := instances.Get(ctx, "environment", "workspace", "config", "1")
		must(t, e)
		if retired.Admission != "retired" {
			t.Fatal("removed replica remained selectable")
		}
	})
}
func wait(t *testing.T, ctx context.Context, condition func() bool) {
	t.Helper()
	for {
		if condition() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("ownership integration timed out")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type workloadAuthority struct{ value client.WorkloadTarget }

func (w *workloadAuthority) Ensure(context.Context, string, string) (client.WorkloadTarget, error) {
	return w.value, nil
}
func (w *workloadAuthority) List(context.Context, string) ([]client.WorkloadTarget, string, error) {
	return []client.WorkloadTarget{w.value}, "", nil
}

type workloadResources struct {
	mu     sync.Mutex
	state  *client.NativeWorkload
	native *nativeFixture
}

func (w *workloadResources) Read(context.Context, contract.Spec) (*client.NativeWorkload, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state == nil {
		return nil, client.ErrWorkloadAbsent
	}
	v := *w.state
	return &v, nil
}
func (w *workloadResources) Save(_ context.Context, v *client.NativeWorkload) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state != nil && w.state.Revision != v.Revision {
		return client.ErrWorkloadConflict
	}
	revision := 1
	if w.state != nil {
		revision, _ = strconv.Atoi(w.state.Revision)
		revision++
	}
	copy := *v
	copy.Revision = strconv.Itoa(revision)
	copy.Settled = true
	w.state = &copy
	w.native.mu.Lock()
	defer w.native.mu.Unlock()
	for ordinal, row := range w.native.rows {
		n, _ := strconv.Atoi(ordinal)
		row.Eligible = n < int(v.Spec.Replicas)
		row.Ready = row.Eligible
		w.native.rows[ordinal] = row
	}
	return nil
}
