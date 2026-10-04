package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	api "media_agent/media_auth/biz/domain/api_token"
)

type agentCandidates struct {
	rows []api.Token
	err  error
}

func (s *agentCandidates) ListAgentRevocationCandidates(_ context.Context, after string, limit int) ([]api.Token, error) {
	if s.err != nil {
		return nil, s.err
	}
	rows := []api.Token{}
	for _, row := range s.rows {
		if row.ID > after {
			rows = append(rows, row)
			if len(rows) == limit {
				break
			}
		}
	}
	return rows, nil
}

type agentAssessment struct {
	tx     *agentCleanupTx
	calls  int
	valid  bool
	err    error
	cancel context.CancelFunc
}

func (a *agentAssessment) Assess(ctx context.Context, _, _, _ string) (bool, error) {
	if a.tx.inside {
		panic("authority lookup inside database transaction")
	}
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded lookup")
	}
	a.calls++
	if a.cancel != nil {
		a.cancel()
	}
	return a.valid, a.err
}

type agentCleanupTx struct{ inside bool }

func (tx *agentCleanupTx) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	tx.inside = true
	defer func() { tx.inside = false }()
	return fn(ctx)
}

type agentCleanup struct {
	tx    *agentCleanupTx
	calls []api.Token
	err   error
}

func (r *agentCleanup) PrepareActorRevocation(_ context.Context, kind api.ActorType, actor, owner, org string) error {
	if !r.tx.inside {
		panic("cleanup intent outside transaction")
	}
	r.calls = append(r.calls, api.Token{ActorType: kind, ActorID: actor, SubjectUserID: owner, OrganizationID: org})
	return r.err
}

func TestAgentAccessRevocationOnlyConfirmedInvalidationWritesIntent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
		err   error
		want  int
	}{
		{"valid", true, nil, 0},
		{"invalid", false, nil, 1},
		{"authority outage", false, errors.New("sensitive upstream failure"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &agentCleanupTx{}
			assessor := &agentAssessment{tx: tx, valid: tc.valid, err: tc.err}
			tokens := &agentCleanup{tx: tx}
			source := &agentCandidates{rows: []api.Token{
				{ID: "a", ActorType: api.ActorAgent, ActorID: "Agent:stable", SubjectUserID: "original-owner", OrganizationID: "original-org"},
				{ID: "b", ActorType: api.ActorPersonalWorker},
			}}
			cursor, err := NewAgentAccessRevocation(source, assessor, tokens, tx).ProcessBatch(t.Context(), "")
			if err != nil || cursor != "" || assessor.calls != 1 || len(tokens.calls) != tc.want {
				t.Fatal("invalid cleanup boundary", err)
			}
			if tc.want == 1 {
				got := tokens.calls[0]
				if got.ActorType != api.ActorAgent || got.ActorID != "Agent:stable" || got.SubjectUserID != "original-owner" || got.OrganizationID != "original-org" {
					t.Fatal("cleanup lost exact stored identity")
				}
			}
		})
	}
}

func TestAgentAccessRevocationPagesPastFailuresAndRestarts(t *testing.T) {
	source := &agentCandidates{}
	for i := 0; i < 101; i++ {
		source.rows = append(source.rows, api.Token{ID: fmt.Sprintf("token-%03d", i), ActorType: api.ActorAgent})
	}
	tx := &agentCleanupTx{}
	assessor := &agentAssessment{tx: tx}
	tokens := &agentCleanup{tx: tx, err: errors.New("write failed")}
	flow := NewAgentAccessRevocation(source, assessor, tokens, tx)
	cursor, err := flow.ProcessBatch(t.Context(), "")
	if err != nil || cursor != "token-099" || len(tokens.calls) != 100 {
		t.Fatal("write failure starved later entries", err)
	}
	cursor, err = flow.ProcessBatch(t.Context(), cursor)
	if err != nil || cursor != "" || len(tokens.calls) != 101 {
		t.Fatal("tail page skipped", err)
	}
	tokens.err, tokens.calls = nil, nil
	if _, err := NewAgentAccessRevocation(source, assessor, tokens, tx).ProcessBatch(t.Context(), ""); err != nil || len(tokens.calls) != 100 {
		t.Fatal("restart did not retry", err)
	}
	source.err = errors.New("scan failed")
	if cursor, err := flow.ProcessBatch(t.Context(), "last"); !errors.Is(err, source.err) || cursor != "last" {
		t.Fatal("scan failure lost cursor")
	}
}

func TestAgentAccessRevocationCancellationDoesNotWriteIntent(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	tx := &agentCleanupTx{}
	assessor := &agentAssessment{tx: tx, cancel: cancel}
	tokens := &agentCleanup{tx: tx}
	source := &agentCandidates{rows: []api.Token{{ID: "a", ActorType: api.ActorAgent}, {ID: "b", ActorType: api.ActorAgent}}}
	flow := NewAgentAccessRevocation(source, assessor, tokens, tx)
	if _, err := flow.ProcessBatch(ctx, ""); !errors.Is(err, context.Canceled) || len(tokens.calls) != 0 {
		t.Fatal("cancelled assessment wrote intent", err)
	}
	if _, err := flow.ProcessBatch(ctx, ""); !errors.Is(err, context.Canceled) || assessor.calls != 1 {
		t.Fatal("cancelled scan called authority", err)
	}
	flow.Run(ctx) // already-cancelled runner must terminate without another lookup
}
