package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_auth/biz/domain/authorizationprojection"
)

type projectionOutboxFake struct {
	events            []domain.Event
	delivered, failed bool
	retryAt           time.Time
}

func (f *projectionOutboxFake) ClaimPending(context.Context, int) ([]domain.Event, error) {
	return f.events, nil
}
func (f *projectionOutboxFake) MarkDelivered(context.Context, domain.Event) error {
	f.delivered = true
	return nil
}
func (f *projectionOutboxFake) MarkFailed(_ context.Context, _ domain.Event, _ string, retryAt time.Time) error {
	f.failed, f.retryAt = true, retryAt
	return nil
}

type projectionSnapshotFake struct {
	snapshot     domain.OrganizationSnapshot
	teamSnapshot domain.TeamSnapshot
}

func (f projectionSnapshotFake) Organization(context.Context, string) (domain.OrganizationSnapshot, error) {
	return f.snapshot, nil
}
func (f projectionSnapshotFake) Team(context.Context, string) (domain.TeamSnapshot, error) {
	return f.teamSnapshot, nil
}

type tupleStoreFake struct {
	tuples map[string]domain.Tuple
	fail   bool
}

func (f *tupleStoreFake) List(context.Context, string, string) ([]domain.Tuple, error) {
	if f.fail {
		return nil, errors.New("keto unavailable")
	}
	out := make([]domain.Tuple, 0, len(f.tuples))
	for _, tuple := range f.tuples {
		out = append(out, tuple)
	}
	return out, nil
}
func (f *tupleStoreFake) Put(_ context.Context, tuple domain.Tuple) error {
	f.tuples[tupleKey(tuple)] = tuple
	return nil
}
func (f *tupleStoreFake) Delete(_ context.Context, tuple domain.Tuple) error {
	delete(f.tuples, tupleKey(tuple))
	return nil
}

func TestAuthorizationProjectionReconcilesOrganizationSnapshot(t *testing.T) {
	event := domain.Event{ID: "e1", AggregateType: "organization", AggregateID: "o1", Sequence: 3}
	outbox := &projectionOutboxFake{events: []domain.Event{event}}
	store := &tupleStoreFake{tuples: map[string]domain.Tuple{
		"Organization:o1#members@removed": {Namespace: "Organization", Object: "o1", Relation: "members", SubjectID: "removed"},
	}}
	worker := NewAuthorizationProjection(outbox, projectionSnapshotFake{snapshot: domain.OrganizationSnapshot{
		ID: "o1", Status: "active",
		Members:      []domain.OrganizationMember{{UserID: "admin", Role: "admin"}, {UserID: "member", Role: "member"}},
		Entitlements: []string{"workflow.run"},
	}}, store)
	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !outbox.delivered || outbox.failed {
		t.Fatalf("delivered=%v failed=%v", outbox.delivered, outbox.failed)
	}
	if len(store.tuples) != 4 {
		t.Fatalf("tuples=%+v", store.tuples)
	}
	if _, ok := store.tuples["Organization:o1#admins@admin"]; !ok {
		t.Fatal("missing admin tuple")
	}
	if _, ok := store.tuples["Organization:o1#workflow_entitled@Organization:o1#members"]; !ok {
		t.Fatal("missing entitlement subject set")
	}
}

func TestAuthorizationProjectionRetriesKetoFailure(t *testing.T) {
	outbox := &projectionOutboxFake{events: []domain.Event{{ID: "e1", AggregateType: "organization", AggregateID: "o1", Attempts: 2}}}
	worker := NewAuthorizationProjection(outbox, projectionSnapshotFake{snapshot: domain.OrganizationSnapshot{ID: "o1", Status: "active"}}, &tupleStoreFake{fail: true})
	before := time.Now()
	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !outbox.failed || outbox.delivered || !outbox.retryAt.After(before.Add(3*time.Second)) {
		t.Fatalf("outbox=%+v", outbox)
	}
}

func TestDisabledOrganizationRemovesAllTuples(t *testing.T) {
	store := &tupleStoreFake{tuples: map[string]domain.Tuple{"Organization:o1#members@u1": {Namespace: "Organization", Object: "o1", Relation: "members", SubjectID: "u1"}}}
	if err := reconcile(context.Background(), store, []domain.Tuple{{Namespace: "Organization", Object: "o1", Relation: "members", SubjectID: "u1"}}, organizationTuples(domain.OrganizationSnapshot{ID: "o1", Status: "disabled"})); err != nil {
		t.Fatal(err)
	}
	if len(store.tuples) != 0 {
		t.Fatalf("tuples=%v", store.tuples)
	}
}

func TestAuthorizationProjectionReconcilesRestrictedTeamSnapshot(t *testing.T) {
	event := domain.Event{ID: "e-team", AggregateType: "team", AggregateID: "t1", Sequence: 4}
	outbox := &projectionOutboxFake{events: []domain.Event{event}}
	store := &tupleStoreFake{tuples: map[string]domain.Tuple{}}
	worker := NewAuthorizationProjection(outbox, projectionSnapshotFake{teamSnapshot: domain.TeamSnapshot{
		ID: "t1", OrganizationID: "o1", Status: "active", EntitlementMode: "restricted",
		Members:                  []domain.TeamMember{{UserID: "admin", Role: "admin"}, {UserID: "member", Role: "member"}},
		OrganizationEntitlements: []string{"materials.read", "workflow.run"},
		TeamEntitlements:         []string{"workflow.run", "outside.invalid"},
	}}, store)
	if err := worker.ProcessPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !outbox.delivered || outbox.failed {
		t.Fatalf("delivered=%v failed=%v", outbox.delivered, outbox.failed)
	}
	if _, ok := store.tuples["Team:t1#organization@o1"]; !ok {
		t.Fatal("missing organization tuple")
	}
	if _, ok := store.tuples["Team:t1#workflow_users@Team:t1#members"]; !ok {
		t.Fatal("missing workflow entitlement")
	}
	if _, ok := store.tuples["Team:t1#materials_users@Team:t1#members"]; ok {
		t.Fatal("restricted team must not inherit unconfigured materials entitlement")
	}
}
