package start_delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

type cancelRepo struct {
	*startRepo
	finished string
}

func (*cancelRepo) CancelInitialStart(context.Context, string) (bool, error) { panic("request only") }
func (r *cancelRepo) FinishCancelledStart(_ context.Context, _ domain.ExecutionStart, id string) error {
	r.finished = id
	*r.events = append(*r.events, "cancel-bind")
	return nil
}

type cleanupEngine struct {
	engine
	status       string
	instanceID   string
	terminateErr error
}

func (e *cleanupEngine) Get(context.Context, string) (*domain.EngineExecution, error) {
	*e.events = append(*e.events, "get")
	return &domain.EngineExecution{Status: e.status, WorkflowInstanceID: e.instanceID}, nil
}

func TestCancelledCleanupRejectsDifferentObservedInstance(t *testing.T) {
	command := &domain.ExecutionStart{ID: "start", JobID: "job", WorkflowName: "frozen", WorkflowVersion: 1, CompilationHash: strings.Repeat("a", 64), CorrelationID: "job", WorkflowInput: []byte(`{"job_id":"job"}`), TaskDomains: []byte(`{}`), Status: "cancel_pending", ClaimToken: "lease", ConductorWorkflowID: "expected"}
	command.PayloadDigest, _ = command.Digest()
	var events []string
	r := &cancelRepo{startRepo: &startRepo{command: command, events: &events}}
	e := &cleanupEngine{engine: engine{events: &events}, status: "RUNNING", instanceID: "different"}
	j := &domain.Job{Status: domain.JobCancelled, Origin: "native", CompilationHash: command.CompilationHash}
	f := Flow{Starts: r, Jobs: jobs{job: j}, Engine: e}
	if err := f.DispatchOne(context.Background()); !errors.Is(err, ErrTemporarilyUnavailable) || r.code != "cancel_execution_identity_conflict" || strings.Join(events, ",") != "get" || r.finished != "" {
		t.Fatal("wrong observed instance was terminated or confirmed", err)
	}
}
func (e *cleanupEngine) Terminate(context.Context, string, string) error {
	*e.events = append(*e.events, "terminate")
	e.status = "TERMINATED" // Upstream commit can precede a lost response.
	return e.terminateErr
}

func TestCancelledStartCleanupNeverLaunchesOrRevivesJob(t *testing.T) {
	for _, tc := range []struct {
		name, status, upstream, result, wantEvents, wantCode string
		lost                                                 bool
	}{
		{name: "bound running", status: "cancel_pending", upstream: "RUNNING", result: "execution", wantEvents: "get,terminate,get,cancel-bind"},
		{name: "uncertain found", status: "uncertain", upstream: "RUNNING", result: "execution", wantEvents: "lookup,get,terminate,get,cancel-bind"},
		{name: "already terminated", status: "cancel_pending", upstream: "TERMINATED", result: "execution", wantEvents: "get,cancel-bind"},
		{name: "unknown not found", status: "uncertain", upstream: "RUNNING", wantEvents: "lookup", wantCode: "cancel_execution_unconfirmed"},
		{name: "termination response lost", status: "cancel_pending", upstream: "RUNNING", result: "execution", lost: true, wantEvents: "get,terminate", wantCode: "cancel_termination_unconfirmed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := &domain.ExecutionStart{ID: "start", JobID: "job", WorkflowName: "frozen", WorkflowVersion: 3, CompilationHash: strings.Repeat("a", 64), CorrelationID: "job", WorkflowInput: []byte(`{"job_id":"job"}`), TaskDomains: []byte(`{}`), Status: tc.status, ClaimToken: "lease"}
			if tc.status == "cancel_pending" {
				command.ConductorWorkflowID = tc.result
			}
			command.PayloadDigest, _ = command.Digest()
			var events []string
			r := &cancelRepo{startRepo: &startRepo{command: command, events: &events}}
			j := &domain.Job{Status: domain.JobCancelled, Origin: "native", CompilationHash: command.CompilationHash}
			e := &cleanupEngine{engine: engine{events: &events, result: tc.result}, status: tc.upstream}
			if tc.lost {
				e.terminateErr = errors.New("private body")
			}
			f := Flow{Starts: r, Jobs: jobs{job: j}, Engine: e}
			err := f.DispatchOne(context.Background())
			if strings.Join(events, ",") != tc.wantEvents || r.code != tc.wantCode || j.Status != domain.JobCancelled {
				t.Fatalf("cleanup events=%v code=%s", events, r.code)
			}
			if tc.wantCode == "" && (err != nil || r.finished != tc.result) {
				t.Fatal("cleanup not confirmed", err)
			}
			if tc.wantCode != "" && !errors.Is(err, ErrTemporarilyUnavailable) {
				t.Fatal("unsafe failure", err)
			}
			if tc.lost {
				// A later claimant observes the commit and never repeats Terminate.
				events = nil
				r.code = ""
				if err := f.DispatchOne(context.Background()); err != nil || strings.Join(events, ",") != "get,cancel-bind" {
					t.Fatal("lost response reissued termination", err)
				}
			}
		})
	}
}
