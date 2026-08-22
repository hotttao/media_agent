package batch_run_dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	taskdomain "media_agent/media_agent_gateway/biz/domain/agent_task"
	domain "media_agent/media_agent_gateway/biz/domain/batch_run"
	taskservice "media_agent/media_agent_gateway/biz/service/agent_task"
)

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

var _ dal.TxRunner = directTx{}

type fakeBatches struct {
	batch *domain.BatchRun
	seen  map[string]bool
	apply int
}

func (f *fakeBatches) Get(context.Context, string) (*domain.BatchRun, error) {
	copy := *f.batch
	copy.Items = append([]domain.Item(nil), f.batch.Items...)
	return &copy, nil
}
func (f *fakeBatches) AttachTask(_ context.Context, _ string, cursor int32, itemKey, taskID string, _ time.Time) error {
	if f.batch.Status == domain.StatusCancelled || f.batch.NextCursor != cursor || f.batch.Items[cursor].ItemKey != itemKey {
		return domain.ErrConflict
	}
	f.batch.Items[cursor].AgentTaskID, f.batch.Items[cursor].Status = taskID, domain.ItemQueued
	f.batch.NextCursor++
	f.batch.Status = domain.StatusRunning
	return nil
}
func (f *fakeBatches) ApplyTaskTerminal(_ context.Context, event domain.TaskTerminal, _ time.Time) error {
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	if f.seen[event.EventID] {
		return nil
	}
	f.seen[event.EventID] = true
	f.apply++
	return nil
}

type fakeTasks struct {
	created   []taskservice.CreateCommand
	cancelled []string
}

func (f *fakeTasks) Create(_ context.Context, command taskservice.CreateCommand) (*taskdomain.Task, error) {
	f.created = append(f.created, command)
	return &taskdomain.Task{ID: "task-" + command.IdempotencyKey, Status: taskdomain.StatusQueued}, nil
}
func (f *fakeTasks) Cancel(_ context.Context, id, _ string) (*taskdomain.Task, error) {
	f.cancelled = append(f.cancelled, id)
	return &taskdomain.Task{ID: id, Status: taskdomain.StatusCancelled}, nil
}

func TestDispatchUsesStableBatchAndItemTaskKeys(t *testing.T) {
	batches := &fakeBatches{batch: &domain.BatchRun{ID: "batch", WorkspaceID: "workspace", RequestedBy: "user", Status: domain.StatusCreating, ItemCount: 2, Items: []domain.Item{{ItemKey: "one", SessionID: "s1", InputJSON: "{}", Status: domain.ItemPending}, {ItemKey: "two", SessionID: "s2", InputJSON: "{}", Status: domain.ItemPending}}}}
	tasks := &fakeTasks{}
	dispatcher := NewDispatcher(batches, directTx{}, tasks)
	if err := dispatcher.Process(context.Background(), "batch_run.dispatch", "batch"); err != nil {
		t.Fatal(err)
	}
	if batches.batch.NextCursor != 2 || batches.batch.Status != domain.StatusRunning || len(tasks.created) != 2 {
		t.Fatalf("batch=%+v created=%+v", batches.batch, tasks.created)
	}
	for i, key := range []string{"one", "two"} {
		command := tasks.created[i]
		if command.Source != "batch_run" || command.SourceResourceID != "batch" || command.IdempotencyKey != key {
			t.Fatalf("command=%+v", command)
		}
	}
}

func TestCancelledBatchPropagatesToLinkedTasks(t *testing.T) {
	batches := &fakeBatches{batch: &domain.BatchRun{ID: "batch", RequestedBy: "user", Status: domain.StatusCancelled, ItemCount: 2, Items: []domain.Item{{ItemKey: "one", AgentTaskID: "task-one", Status: domain.ItemQueued}, {ItemKey: "two", Status: domain.ItemCancelled}}}}
	tasks := &fakeTasks{}
	if err := NewDispatcher(batches, directTx{}, tasks).Process(context.Background(), "batch_run.cancel", "batch"); err != nil {
		t.Fatal(err)
	}
	if len(tasks.cancelled) != 1 || tasks.cancelled[0] != "task-one" {
		t.Fatalf("cancelled=%v", tasks.cancelled)
	}
}

type fakeSource struct {
	event     domain.TaskTerminal
	delivered bool
	failMark  bool
}

func (f *fakeSource) ListPendingBatchTerminal(context.Context, int) ([]domain.TaskTerminal, error) {
	if f.delivered {
		return nil, nil
	}
	return []domain.TaskTerminal{f.event}, nil
}
func (f *fakeSource) MarkDelivered(context.Context, string, time.Time) error {
	if f.failMark {
		f.failMark = false
		return errors.New("commit outcome unknown")
	}
	f.delivered = true
	return nil
}

func TestProjectorInboxMakesRepeatedTerminalDeliveryHarmless(t *testing.T) {
	batches := &fakeBatches{}
	source := &fakeSource{event: domain.TaskTerminal{EventID: "event", TaskID: "task", Status: "completed"}, failMark: true}
	projector := NewProjector(source, batches, directTx{})
	if err := projector.Flush(context.Background(), 10); err == nil {
		t.Fatal("expected delivery mark failure")
	}
	if err := projector.Flush(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if batches.apply != 1 || !source.delivered {
		t.Fatalf("apply=%d delivered=%v", batches.apply, source.delivered)
	}
}
