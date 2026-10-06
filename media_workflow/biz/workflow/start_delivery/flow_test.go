package start_delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

type startRepo struct {
	domain.ExecutionStartRepo
	command            *domain.ExecutionStart
	events             *[]string
	markErr, finishErr error
	code               string
}

func (r *startRepo) ClaimStart(context.Context) (*domain.ExecutionStart, error) {
	return r.command, nil
}
func (r *startRepo) MarkStartSent(context.Context, domain.ExecutionStart) error {
	*r.events = append(*r.events, "mark")
	return r.markErr
}
func (r *startRepo) FinishStart(_ context.Context, _ domain.ExecutionStart, id string) error {
	*r.events = append(*r.events, "bind:"+id)
	return r.finishErr
}
func (r *startRepo) ReleaseStartClaim(_ context.Context, _ domain.ExecutionStart, code string) error {
	r.code = code
	return nil
}

type jobs struct {
	domain.JobRepo
	job *domain.Job
}

func (j jobs) Get(context.Context, string) (*domain.Job, error) { return j.job, nil }

type engine struct {
	events *[]string
	result string
	err    error
}

func (e engine) Start(_ context.Context, name string, version int, correlation string, input map[string]any, domains map[string]string) (string, error) {
	if name != "frozen" || version != 3 || correlation != "job" || input["job_id"] != "job" || domains["capability"] != "personal" {
		panic("frozen payload changed")
	}
	*e.events = append(*e.events, "post")
	return e.result, e.err
}
func (e engine) FindStartedExecution(_ context.Context, name, correlation string) (string, error) {
	if name != "frozen" || correlation != "job" {
		panic("lookup identity changed")
	}
	*e.events = append(*e.events, "lookup")
	return e.result, e.err
}

func TestStartDeliveryNeverResendsUncertainCommands(t *testing.T) {
	privateErr := errors.New("private upstream response")
	for _, tc := range []struct {
		name, status, result, wantEvents, wantCode string
		markErr, finishErr, engineErr              error
		cancelled, corrupt                         bool
	}{
		{name: "queued launch", status: "queued", result: "execution", wantEvents: "mark,post,bind:execution"},
		{name: "uncertain recovery", status: "uncertain", result: "execution", wantEvents: "lookup,bind:execution"},
		{name: "empty search is not resend permission", status: "uncertain", wantEvents: "lookup", wantCode: "start_outcome_unconfirmed"},
		{name: "lookup unavailable", status: "uncertain", engineErr: privateErr, wantEvents: "lookup", wantCode: "start_outcome_unconfirmed"},
		{name: "post response lost", status: "queued", engineErr: privateErr, wantEvents: "mark,post", wantCode: "start_outcome_unconfirmed"},
		{name: "binding failed", status: "queued", result: "execution", finishErr: privateErr, wantEvents: "mark,post,bind:execution", wantCode: "start_binding_unavailable"},
		{name: "mark rejected", status: "queued", markErr: domain.ErrStartConflict, wantEvents: "mark", wantCode: "start_mark_unavailable"},
		{name: "cancelled queued", status: "queued", cancelled: true, wantCode: "job_no_longer_pending"},
		{name: "cancelled uncertain", status: "uncertain", cancelled: true, wantCode: "job_no_longer_pending"},
		{name: "digest changed", status: "queued", corrupt: true, wantCode: "invalid_frozen_start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := &domain.ExecutionStart{ID: "start", JobID: "job", WorkflowName: "frozen", WorkflowVersion: 3, CorrelationID: "job", CompilationHash: strings.Repeat("a", 64), WorkflowInput: []byte(`{"job_id":"job"}`), TaskDomains: []byte(`{"capability":"personal"}`), ClaimToken: "lease", Status: tc.status}
			command.PayloadDigest, _ = command.Digest()
			if tc.corrupt {
				command.WorkflowVersion++
			}
			job := &domain.Job{Status: domain.JobPending, Origin: "native", CompilationHash: command.CompilationHash}
			if tc.cancelled {
				job.Status = domain.JobCancelled
			}
			var events []string
			r := &startRepo{command: command, events: &events, markErr: tc.markErr, finishErr: tc.finishErr}
			f := Flow{Starts: r, Jobs: jobs{job: job}, Engine: engine{events: &events, result: tc.result, err: tc.engineErr}}
			err := f.DispatchOne(context.Background())
			if strings.Join(events, ",") != tc.wantEvents || r.code != tc.wantCode {
				t.Fatalf("events=%v code=%s", events, r.code)
			}
			if tc.wantCode == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantCode != "" && !errors.Is(err, ErrTemporarilyUnavailable) {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}
