package teamwork_runtime_change

import (
	"context"
	"testing"
	"time"

	"media_agent/media_agent_gateway/biz/dal"
	taskdomain "media_agent/media_agent_gateway/biz/domain/agent_task"
	domain "media_agent/media_agent_gateway/biz/domain/teamwork_runtime"
)

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

var _ dal.TxRunner = directTx{}

type versionRepo struct{ result domain.ApplyResult }

func (f *versionRepo) ApplyEvent(context.Context, domain.Event, time.Time) (domain.ApplyResult, error) {
	return f.result, nil
}
func (f *versionRepo) Get(context.Context, string) (*domain.Version, error) {
	return nil, domain.ErrNotFound
}

type taskCanceller struct {
	tasks []taskdomain.Task
	calls int
}

func (f *taskCanceller) CancelStaleTopology(context.Context, string, int64, time.Time) ([]taskdomain.Task, error) {
	f.calls++
	return f.tasks, nil
}

type taskOutbox struct{ events []taskdomain.OutboxEvent }

func (f *taskOutbox) Enqueue(_ context.Context, event *taskdomain.OutboxEvent) error {
	f.events = append(f.events, *event)
	return nil
}
func (f *taskOutbox) ListPendingActions(context.Context, int) ([]taskdomain.OutboxEvent, error) {
	return nil, nil
}
func (f *taskOutbox) MarkDelivered(context.Context, string, time.Time) error              { return nil }
func (f *taskOutbox) MarkDeliveryFailed(context.Context, string, string, time.Time) error { return nil }

func TestApplyAdvancedVersionCancelsQueuedTasks(t *testing.T) {
	versions := &versionRepo{result: domain.ApplyResult{Advanced: true}}
	tasks := &taskCanceller{tasks: []taskdomain.Task{{ID: "task-1"}, {ID: "task-2"}}}
	outbox := &taskOutbox{}
	result, err := New(versions, tasks, outbox, directTx{}).Apply(context.Background(), "event-1", "workspace-1", "topology-1", 2, "active")
	if err != nil || !result.Applied || result.CancelledTaskCount != 2 || tasks.calls != 1 || len(outbox.events) != 2 {
		t.Fatalf("result=%+v calls=%d events=%+v err=%v", result, tasks.calls, outbox.events, err)
	}
}

func TestApplyDuplicateDoesNotCancelAgain(t *testing.T) {
	tasks := &taskCanceller{tasks: []taskdomain.Task{{ID: "task-1"}}}
	result, err := New(&versionRepo{result: domain.ApplyResult{Duplicate: true}}, tasks, &taskOutbox{}, directTx{}).Apply(context.Background(), "event-1", "workspace-1", "topology-1", 2, "active")
	if err != nil || result.Applied || tasks.calls != 0 {
		t.Fatalf("result=%+v calls=%d err=%v", result, tasks.calls, err)
	}
}
