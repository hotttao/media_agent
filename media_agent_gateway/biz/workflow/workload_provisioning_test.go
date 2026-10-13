package workflow

import (
	"context"
	"errors"
	contract "media_agent/hertz_infra/agentworkload"
	instance "media_agent/media_agent_gateway/biz/domain/agent_instance"
	session "media_agent/media_agent_gateway/biz/domain/agent_session"
	instances "media_agent/media_agent_gateway/biz/service/agent_instance"
	"media_agent/media_agent_gateway/biz/shared/client"
	"strconv"
	"sync"
	"testing"
)

type workloadControlFixture struct{ target WorkloadTarget }

func (f *workloadControlFixture) Ensure(context.Context, string, string) (WorkloadTarget, error) {
	return f.target, nil
}
func (f *workloadControlFixture) List(context.Context, string) ([]WorkloadTarget, string, error) {
	return []WorkloadTarget{f.target}, "", nil
}

type workloadResourceFixture struct {
	mu     sync.Mutex
	state  *NativeWorkload
	writes []NativeWorkload
}

func (f *workloadResourceFixture) Read(context.Context, contract.Spec) (*NativeWorkload, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state == nil {
		return nil, ErrWorkloadAbsent
	}
	v := *f.state
	return &v, nil
}
func (f *workloadResourceFixture) Save(_ context.Context, v *NativeWorkload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != nil && f.state.Revision != v.Revision || f.state == nil && v.Revision != "" {
		return ErrWorkloadConflict
	}
	n := 1
	if f.state != nil {
		n, _ = strconv.Atoi(f.state.Revision)
		n++
	}
	next := *v
	next.Settled = true
	next.Revision = strconv.Itoa(n)
	f.state = &next
	f.writes = append(f.writes, next)
	return nil
}

type workloadInstanceFixture struct {
	mu   sync.Mutex
	rows map[string]instance.Instance
}

func (f *workloadInstanceFixture) Get(_ context.Context, _, _, _, replica string, _ bool) (*instance.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.rows[replica]
	if !ok {
		return nil, instance.ErrNotFound
	}
	return &v, nil
}
func (f *workloadInstanceFixture) Historical(ctx context.Context, a, b, c, d, e string) (*instance.Instance, error) {
	return f.Get(ctx, a, b, c, d, false)
}
func (f *workloadInstanceFixture) List(context.Context, string, string, string) ([]instance.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []instance.Instance{}
	for _, v := range f.rows {
		out = append(out, v)
	}
	return out, nil
}
func (f *workloadInstanceFixture) Save(_ context.Context, v *instance.Instance, expected int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if old, ok := f.rows[v.ReplicaID]; ok && old.Version != expected {
		return instance.ErrConflict
	}
	f.rows[v.ReplicaID] = *v
	return nil
}
func (f *workloadInstanceFixture) Drain(_ context.Context, v *instance.Instance, operation string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v.Version++
	v.Admission = "draining"
	v.DrainOperationID = operation
	f.rows[v.ReplicaID] = *v
	return nil
}
func (f *workloadInstanceFixture) Retire(_ context.Context, v *instance.Instance, operation string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v.Version++
	v.Admission = "retired"
	f.rows[v.ReplicaID] = *v
	return nil
}

type workloadTxFixture struct{ mu sync.Mutex }

func (t *workloadTxFixture) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fn(ctx)
}

type workloadBindingFixture struct {
	session.BindingRepository
	pending []session.Session
}

func (f workloadBindingFixture) BoundSessions(context.Context, string, int) ([]session.Session, error) {
	return nil, nil
}
func (f workloadBindingFixture) PendingTransfers(context.Context, int) ([]session.Session, error) {
	return f.pending, nil
}

type workloadRuntimeFixture struct {
	client.ControlledRuntime
	mu             sync.Mutex
	running        int64
	unknown        bool
	operation, inc string
}

func (r *workloadRuntimeFixture) BeginDrain(_ context.Context, op string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operation = op
	return nil
}
func (r *workloadRuntimeFixture) GetDrain(context.Context) (client.DrainState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.unknown {
		return client.DrainState{}, errors.New("unreachable Agent")
	}
	return client.DrainState{OperationID: r.operation, IncarnationID: r.inc, RunningCount: r.running, Quiesced: r.running == 0}, nil
}

type workloadFactoryFixture struct{ peer *workloadRuntimeFixture }

func (f workloadFactoryFixture) ForInstance(instance.Instance, int64) client.ControlledRuntime {
	return f.peer
}

type workloadNativeFixture struct {
	instance.ResourceAuthority
	terminated bool
}

func (f workloadNativeFixture) Terminated(context.Context, instance.Instance) (bool, error) {
	return f.terminated, nil
}
func (f workloadNativeFixture) Observe(_ context.Context, v instance.Instance) (instance.ResourceFacts, error) {
	return instance.ResourceFacts{Locator: instance.ResourceLocator{ResourceUID: v.ResourceUID}, Generation: v.ResourceGeneration, Eligible: true, Ready: true, Running: true}, nil
}
func workloadFixture() (*WorkloadProvisioning, *workloadResourceFixture, *workloadRuntimeFixture, *workloadControlFixture) {
	spec := contract.Spec{Driver: "kubernetes", Namespace: "agents", EnvironmentID: "env", WorkspaceID: "ws", AgentConfigID: "cfg", AgentType: "codex", Replicas: 1}
	control := &workloadControlFixture{target: WorkloadTarget{Spec: spec, Version: 1, OwnerUserID: "owner"}}
	resources := &workloadResourceFixture{}
	repo := &workloadInstanceFixture{rows: map[string]instance.Instance{"0": {EnvironmentID: "env", WorkspaceID: "ws", AgentConfigID: "cfg", ReplicaID: "0", IncarnationID: "inc", Admission: "active", Version: 1}}}
	runtime := &workloadRuntimeFixture{inc: "inc"}
	w := &WorkloadProvisioning{Control: control, Resources: resources, EnvironmentID: "env", Instances: instances.New(repo, &workloadTxFixture{}), Bindings: workloadBindingFixture{}, Factory: workloadFactoryFixture{runtime}, Native: workloadNativeFixture{}}
	return w, resources, runtime, control
}
func TestConcurrentGatewaysDrainBeforeNativeZeroAndRecover(t *testing.T) {
	ctx := context.Background()
	first, resources, peer, control := workloadFixture()
	second := *first
	if e := first.Ensure(ctx, "owner", "ws"); e != nil {
		t.Fatal(e)
	}
	control.target.Spec.Replicas = 0
	control.target.Version = 2
	peer.running = 1
	var wg sync.WaitGroup
	for _, gateway := range []*WorkloadProvisioning{first, &second} {
		wg.Go(func() {
			e := gateway.Ensure(ctx, "owner", "ws")
			if !errors.Is(e, ErrRuntimeWaiting) && !errors.Is(e, ErrWorkloadConflict) {
				t.Errorf("expected pending/conflict: %v", e)
			}
		})
	}
	wg.Wait()
	state, _ := resources.Read(ctx, control.target.Spec)
	if state.Operation == "" || state.Spec.Replicas != 1 {
		t.Fatal("running Turn lost native replica")
	}
	peer.mu.Lock()
	peer.running = 0
	peer.unknown = true
	peer.mu.Unlock()
	if e := second.Ensure(ctx, "owner", "ws"); !errors.Is(e, ErrRuntimeWaiting) {
		t.Fatal("unknown state allowed native removal")
	}
	state, _ = resources.Read(ctx, control.target.Spec)
	if state.Spec.Replicas != 1 {
		t.Fatal("unknown execution removed")
	}
	peer.mu.Lock()
	peer.unknown = false
	peer.mu.Unlock()
	if e := second.Ensure(ctx, "owner", "ws"); !errors.Is(e, ErrRuntimeWaiting) {
		t.Fatalf("commit must preserve pending retirement: %v", e)
	}
	state, _ = resources.Read(ctx, control.target.Spec)
	if state.Spec.Replicas != 0 || state.Version != 2 || state.Operation == "" {
		t.Fatal("drained native intent not committed durably")
	}
	if e := first.Ensure(ctx, "owner", "ws"); e != nil {
		t.Fatal(e)
	}
	state, _ = resources.Read(ctx, control.target.Spec)
	if state.Operation != "" {
		t.Fatal("retirement not recovered")
	}
	v, e := first.Instances.Get(ctx, "env", "ws", "cfg", "0")
	if e != nil || v.Admission != "retired" {
		t.Fatal("retired source selectable")
	}
	control.target.Spec.Replicas = 2
	control.target.Version = 3
	if e = second.Ensure(ctx, "owner", "ws"); e != nil {
		t.Fatal(e)
	}
	state, _ = resources.Read(ctx, control.target.Spec)
	if state.Spec.Replicas != 2 || state.Version != 3 {
		t.Fatal("growth not applied")
	}
	v.IncarnationID = "new-inc"
	v.ResourceUID = "new-pod"
	v.ResourceGeneration = "new-container"
	v.Endpoint = "http://agent"
	v.CredentialHash = "hash"
	v.CredentialKeyID = "key"
	v, e = first.Instances.Register(ctx, *v, v.Version)
	if e != nil || v.Admission != "active" || v.DrainOperationID != "" {
		t.Fatalf("new native generation did not reactivate retired ordinal: %v", e)
	}
}
func TestPendingTransferPreventsCommitAndSupersededDrainFinishes(t *testing.T) {
	ctx := context.Background()
	w, resources, _, control := workloadFixture()
	if e := w.Ensure(ctx, "owner", "ws"); e != nil {
		t.Fatal(e)
	}
	control.target.Spec.Replicas = 0
	control.target.Version = 2
	_ = w.Ensure(ctx, "owner", "ws")
	w.Bindings = workloadBindingFixture{pending: []session.Session{{EnvironmentID: "env", WorkspaceID: "ws", AgentConfigID: "cfg", BindingPhase: "transferring"}}}
	if e := w.Ensure(ctx, "owner", "ws"); !errors.Is(e, ErrRuntimeWaiting) {
		t.Fatal("uncompleted binding did not block")
	}
	state, _ := resources.Read(ctx, control.target.Spec)
	if state.Spec.Replicas != 1 {
		t.Fatal("committed count before transfer")
	}
	control.target.Spec.Replicas = 3
	control.target.Version = 3
	w.Bindings = workloadBindingFixture{}
	_ = w.Ensure(ctx, "owner", "ws")
	if e := w.Ensure(ctx, "owner", "ws"); e != nil {
		t.Fatal(e)
	}
	if e := w.Ensure(ctx, "owner", "ws"); e != nil {
		t.Fatal(e)
	}
	state, _ = resources.Read(ctx, control.target.Spec)
	if state.Spec.Replicas != 3 || state.Version != 3 {
		t.Fatal("superseding Control version lost")
	}
}
func TestMembershipIncludesMissingOrdinal(t *testing.T) {
	ctx := context.Background()
	w, _, _, control := workloadFixture()
	control.target.Spec.Replicas = 2
	result, complete, e := w.Membership(ctx, "ws")
	if e != nil || !complete || len(result) != 2 || result[1].IncarnationID != "" {
		t.Fatalf("missing replica omitted: %+v %v", result, e)
	}
}
