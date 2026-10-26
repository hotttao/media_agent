package execute

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"media_agent/media_worker/biz/domain/execution"
)

type memoryResults struct {
	pending                        []execution.Result
	events                         *[]string
	saveErr, pendingErr, finishErr error
	rejected                       bool
}

func (r *memoryResults) Save(v execution.Result) error {
	*r.events = append(*r.events, "save")
	if r.saveErr != nil {
		return r.saveErr
	}
	r.pending = append(r.pending, v)
	return nil
}
func (r *memoryResults) Pending() ([]execution.Result, error) {
	*r.events = append(*r.events, "pending")
	return r.pending, r.pendingErr
}
func (r *memoryResults) Finish(v execution.Result, rejected bool) error {
	*r.events = append(*r.events, "finish")
	r.rejected = rejected
	if r.finishErr != nil {
		return r.finishErr
	}
	r.pending = nil
	return nil
}

type fakeGateway struct {
	events                 *[]string
	task                   *execution.Task
	cancel                 context.CancelFunc
	reportErr, registerErr error
	delivered              execution.Result
}

func (g *fakeGateway) Register(context.Context, string, []execution.Capability) error {
	*g.events = append(*g.events, "register")
	return g.registerErr
}
func (g *fakeGateway) Poll(ctx context.Context, _ string, _ execution.Capability) (*execution.Task, error) {
	*g.events = append(*g.events, "poll")
	task := g.task
	g.task = nil
	if task == nil {
		g.cancel()
	}
	return task, nil
}
func (g *fakeGateway) Heartbeat(context.Context, string, execution.Task) error { return nil }
func (g *fakeGateway) Complete(_ context.Context, _ string, task execution.Task, key string, out map[string]any) (*execution.ResultReceipt, error) {
	*g.events = append(*g.events, "complete")
	g.delivered = execution.Result{Task: task, Key: key, Output: out}
	g.cancel()
	return &execution.ResultReceipt{Status: "submitted"}, g.reportErr
}
func (g *fakeGateway) FailWithKind(_ context.Context, _ string, task execution.Task, key, reason, kind string) (*execution.ResultReceipt, error) {
	*g.events = append(*g.events, "fail")
	g.delivered = execution.Result{Task: task, Key: key, Reason: reason, FailureKind: kind}
	g.cancel()
	return &execution.ResultReceipt{Status: "submitted"}, g.reportErr
}

type fakeExecutor struct {
	events *[]string
	err    error
}

func (e fakeExecutor) AvailableCapabilities() []execution.Capability {
	return []execution.Capability{{Key: "cap", Version: 1, Concurrency: 1}}
}
func (e fakeExecutor) ExecuteContext(context.Context, execution.Task) (map[string]any, error) {
	*e.events = append(*e.events, "execute")
	return map[string]any{"candidates": []any{"result"}}, e.err
}

type transientFailure struct{}

func (transientFailure) Error() string   { return "provider unavailable" }
func (transientFailure) Permanent() bool { return false }

func TestRecoveryUsesOriginalIdentityWithoutExecutingOrPolling(t *testing.T) {
	events := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := execution.Result{Task: execution.Task{DispatchID: "old", Attempt: 3}, Key: "original-key", Output: map[string]any{"candidates": []any{"saved"}}}
	store := &memoryResults{events: &events, pending: []execution.Result{original}}
	gateway := &fakeGateway{events: &events, cancel: cancel}
	err := (Workflow{Gateway: gateway, Executor: fakeExecutor{events: &events}, Results: store}).Run(ctx, Command{WorkerID: "worker", Capabilities: []execution.Capability{{Key: "cap", Version: 1, Concurrency: 1}}})
	if err != nil || !reflect.DeepEqual(events, []string{"register", "pending", "complete", "finish"}) || !reflect.DeepEqual(gateway.delivered, original) {
		t.Fatalf("recovery events=%v delivered=%v err=%v", events, gateway.delivered, err)
	}
}
func TestSavePrecedesSendAndClassifiesFailure(t *testing.T) {
	for _, failure := range []error{nil, transientFailure{}, errors.New("terminal")} {
		t.Run(kindName(failure), func(t *testing.T) {
			events := []string{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store := &memoryResults{events: &events}
			gateway := &fakeGateway{events: &events, cancel: cancel, task: &execution.Task{DispatchID: "dispatch", Attempt: 2}}
			executor := fakeExecutor{events: &events, err: failure}
			err := (Workflow{Gateway: gateway, Executor: executor, Results: store}).Run(ctx, Command{WorkerID: "worker", Capabilities: executor.AvailableCapabilities()})
			method := "complete"
			suffix := ":complete"
			kind := ""
			if failure != nil {
				method = "fail"
				suffix = ":fail"
				kind = kindName(failure)
			}
			if err != nil || !reflect.DeepEqual(events, []string{"register", "pending", "poll", "execute", "save", method, "finish"}) || gateway.delivered.Key != "dispatch:2"+suffix || gateway.delivered.FailureKind != kind {
				t.Fatalf("events=%v delivery=%v err=%v", events, gateway.delivered, err)
			}
		})
	}
}
func kindName(err error) string {
	if err == nil {
		return "success"
	}
	if _, ok := err.(transientFailure); ok {
		return "transient"
	}
	return "permanent"
}
func TestStorageErrorsReturnToCallerBeforeFurtherSideEffects(t *testing.T) {
	for _, step := range []string{"register", "pending", "save", "finish"} {
		t.Run(step, func(t *testing.T) {
			events := []string{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sentinel := errors.New("storage failed")
			store := &memoryResults{events: &events}
			gateway := &fakeGateway{events: &events, cancel: cancel, task: &execution.Task{DispatchID: "dispatch", Attempt: 1}}
			switch step {
			case "register":
				gateway.registerErr = sentinel
			case "pending":
				store.pendingErr = sentinel
			case "save":
				store.saveErr = sentinel
			case "finish":
				store.finishErr = sentinel
			}
			executor := fakeExecutor{events: &events}
			err := (Workflow{Gateway: gateway, Executor: executor, Results: store}).Run(ctx, Command{Capabilities: executor.AvailableCapabilities()})
			if !errors.Is(err, sentinel) || events[len(events)-1] != step {
				t.Fatalf("events=%v err=%v", events, err)
			}
		})
	}
}
func TestRejectedResultsAreArchivedAndUncertainResultsRemainPending(t *testing.T) {
	for _, rejected := range []bool{true, false} {
		events := []string{}
		ctx, cancel := context.WithCancel(context.Background())
		result := execution.Result{Task: execution.Task{DispatchID: "dispatch", Attempt: 1}, Key: "key", Output: map[string]any{"candidates": []any{}}}
		store := &memoryResults{events: &events, pending: []execution.Result{result}}
		gateway := &fakeGateway{events: &events, cancel: cancel, reportErr: errors.New("response lost")}
		if rejected {
			gateway.reportErr = execution.ErrResultRejected
		}
		ok, err := DeliverStoredResult(ctx, gateway, "worker", store, result)
		cancel()
		if ok || err != nil || store.rejected != rejected || (!rejected && len(store.pending) != 1) {
			t.Fatalf("events=%v rejected=%v err=%v", events, store.rejected, err)
		}
	}
}

type recoveryFake struct {
	events *[]string
	tasks  []execution.Task
}

func (r recoveryFake) PendingTasks(context.Context) ([]execution.Task, error) {
	*r.events = append(*r.events, "recover-pending")
	return r.tasks, nil
}
func (r recoveryFake) FinishTask(context.Context, execution.Task) error {
	*r.events = append(*r.events, "recover-finish")
	return nil
}

type admissionFake struct {
	events *[]string
	err    error
}

func (a admissionFake) Acquire(context.Context) (func(), error) {
	*a.events = append(*a.events, "admission")
	return func() { *a.events = append(*a.events, "release") }, a.err
}
func (a admissionFake) AcquireRecovery(context.Context) (func(), error) {
	*a.events = append(*a.events, "recovery-admission")
	return func() { *a.events = append(*a.events, "release") }, nil
}
func TestCapacityIsAcquiredBeforePoll(t *testing.T) {
	events := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sentinel := errors.New("studio busy")
	store := &memoryResults{events: &events}
	gateway := &fakeGateway{events: &events, cancel: cancel}
	executor := fakeExecutor{events: &events}
	err := (Workflow{Gateway: gateway, Executor: executor, Results: store, Admission: admissionFake{events: &events, err: sentinel}}).Run(ctx, Command{Capabilities: executor.AvailableCapabilities()})
	if !errors.Is(err, sentinel) || !reflect.DeepEqual(events, []string{"register", "pending", "admission"}) {
		t.Fatalf("poll occurred without capacity: %v %v", events, err)
	}
}
func TestRecoveryUsesOriginalTaskAndRecoveryCapacityBeforePolling(t *testing.T) {
	events := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := execution.Task{DispatchID: "old-dispatch", Attempt: 4, CapabilityKey: "cap", CapabilityVersion: 1}
	store := &memoryResults{events: &events}
	gateway := &fakeGateway{events: &events, cancel: cancel}
	executor := fakeExecutor{events: &events}
	recovery := recoveryFake{events: &events, tasks: []execution.Task{task}}
	err := (Workflow{Gateway: gateway, Executor: executor, Results: store, Recovery: recovery, Admission: admissionFake{events: &events}}).Run(ctx, Command{Capabilities: executor.AvailableCapabilities()})
	if err != nil || !reflect.DeepEqual(events, []string{"register", "pending", "recover-pending", "recovery-admission", "execute", "save", "complete", "finish", "release"}) || gateway.delivered.Task.DispatchID != "old-dispatch" || gateway.delivered.Key != "old-dispatch:4:complete" {
		t.Fatalf("recovery changed original attempt: %v %v", events, err)
	}
}
