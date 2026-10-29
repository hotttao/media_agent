package job

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	domain "media_agent/media_workflow/biz/domain/job"
)

// Stored cancellation wins over the fetched engine snapshot. List returns
// copies, just as a real repository does, so parallel readers share no rows.
type listProjectionRepo struct {
	domain.JobRepo
	items []*domain.Job
	query domain.ListQuery
}

func (r *listProjectionRepo) List(_ context.Context, query domain.ListQuery) ([]*domain.Job, int, error) {
	r.query = query
	items := make([]*domain.Job, len(r.items))
	for i, item := range r.items {
		copy := *item
		items[i] = &copy
	}
	return items, 91, nil
}

func (r *listProjectionRepo) Get(_ context.Context, id string) (*domain.Job, error) {
	for _, item := range r.items {
		if item.ID == id {
			copy := *item
			copy.Status = domain.JobCancelled
			return &copy, nil
		}
	}
	return nil, errors.New("missing test row")
}

type listProjectionEngine struct {
	domain.WorkflowEngine
	started                chan string
	release                chan struct{}
	failed                 string
	mu                     sync.Mutex
	active, maximum, calls int
}

func (e *listProjectionEngine) Get(ctx context.Context, id string) (*domain.EngineExecution, error) {
	e.mu.Lock()
	e.active++
	e.calls++
	e.maximum = max(e.maximum, e.active)
	e.mu.Unlock()
	defer func() { e.mu.Lock(); e.active--; e.mu.Unlock() }()
	e.started <- id
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.release:
	}
	if id == e.failed {
		return nil, errors.New("unavailable test engine snapshot")
	}
	return &domain.EngineExecution{WorkflowInstanceID: id, Status: "RUNNING"}, nil
}

func projectionListFixture() (*Service, *listProjectionRepo, *listProjectionEngine) {
	repo := &listProjectionRepo{}
	for index := range 9 {
		repo.items = append(repo.items, &domain.Job{ID: fmt.Sprint(index), ConductorWorkflowID: fmt.Sprint(index), Status: domain.JobRunning})
	}
	repo.items[8].ConductorWorkflowID = "" // Imported history has no native instance.
	engine := &listProjectionEngine{started: make(chan string, 9), release: make(chan struct{}), failed: "3"}
	return &Service{jobRepo: repo, engine: engine}, repo, engine
}

func awaitProjectionStarts(t *testing.T, engine *listProjectionEngine) {
	t.Helper()
	for range 4 {
		select {
		case <-engine.started:
		case <-time.After(5 * time.Second):
			t.Fatal("independent engine reads did not overlap")
		}
	}
}

func TestListProjectionOverlapsIndependentReadsWithoutReorderingOrHidingErrors(t *testing.T) {
	svc, repo, engine := projectionListFixture()
	query := domain.ListQuery{SubmittedBy: "owner", OrganizationID: "org", WorkspaceID: "workspace", Limit: 9}
	done := make(chan struct{})
	var items []*domain.Job
	var total int
	var err error
	go func() { items, total, err = svc.List(context.Background(), query); close(done) }()
	awaitProjectionStarts(t, engine)
	close(engine.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("list did not settle after reads were released")
	}
	if err != nil || total != 91 || len(items) != 9 || !reflect.DeepEqual(repo.query, query) {
		t.Fatal("list changed authority filters, pagination or availability", err)
	}
	for index, item := range items {
		want := domain.JobCancelled
		if index == 3 || index == 8 {
			want = domain.JobRunning
		}
		if item.ID != fmt.Sprint(index) || item.Status != want {
			t.Fatal("projection reordered rows, replaced failed snapshots or invented a history instance")
		}
	}
	if engine.maximum != 4 || engine.calls != 8 {
		t.Fatal("projection fanout is not bounded or skipped a native instance")
	}
}

func TestListProjectionCancellationStopsFurtherEngineReads(t *testing.T) {
	svc, _, engine := projectionListFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { _, _, _ = svc.List(ctx, domain.ListQuery{}); close(done) }()
	awaitProjectionStarts(t, engine)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled list retained blocked readers")
	}
	if engine.calls != 4 || engine.active != 0 {
		t.Fatal("cancelled page kept reading remaining executions")
	}
}
