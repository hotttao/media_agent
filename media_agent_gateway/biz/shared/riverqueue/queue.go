package riverqueue

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

const queueName = "agent_tasks"

type AgentTaskArgs struct {
	Action string `json:"action" river:"unique"`
	TaskID string `json:"task_id" river:"unique"`
}

func (AgentTaskArgs) Kind() string { return "agent_task" }

type AgentTaskWorker struct {
	river.WorkerDefaults[AgentTaskArgs]
	work func(context.Context, string, string) error
}

type BatchRunArgs struct {
	Action     string `json:"action" river:"unique"`
	BatchRunID string `json:"batch_run_id" river:"unique"`
}

func (BatchRunArgs) Kind() string { return "batch_run" }

type BatchRunWorker struct {
	river.WorkerDefaults[BatchRunArgs]
	work func(context.Context, string, string) error
}

func (w *BatchRunWorker) Work(ctx context.Context, job *river.Job[BatchRunArgs]) error {
	return w.work(ctx, job.Args.Action, job.Args.BatchRunID)
}

type Handlers struct {
	AgentTask func(context.Context, string, string) error
	BatchRun  func(context.Context, string, string) error
}

func (w *AgentTaskWorker) Work(ctx context.Context, job *river.Job[AgentTaskArgs]) error {
	return w.work(ctx, job.Args.Action, job.Args.TaskID)
}

type Queue struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
}

func New(ctx context.Context, dsn string, migrate bool, handlers Handlers) (*Queue, error) {
	if handlers.AgentTask == nil || handlers.BatchRun == nil {
		return nil, errors.New("riverqueue: Agent Task and Batch Run workers are required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	driver := riverpgxv5.New(pool)
	if migrate {
		migrator, err := rivermigrate.New(driver, nil)
		if err != nil {
			pool.Close()
			return nil, err
		}
		if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			pool.Close()
			return nil, err
		}
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &AgentTaskWorker{work: handlers.AgentTask})
	river.AddWorker(workers, &BatchRunWorker{work: handlers.BatchRun})
	client, err := river.NewClient(driver, &river.Config{
		Queues:  map[string]river.QueueConfig{queueName: {MaxWorkers: 8}},
		Workers: workers,
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &Queue{pool: pool, client: client}, nil
}

func (q *Queue) Start(ctx context.Context) error { return q.client.Start(ctx) }

func (q *Queue) Publish(ctx context.Context, action, taskID string) error {
	_, err := q.client.Insert(ctx, AgentTaskArgs{Action: action, TaskID: taskID}, &river.InsertOpts{
		Queue: queueName,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	})
	return err
}

func (q *Queue) PublishBatch(ctx context.Context, action, batchRunID string) error {
	_, err := q.client.Insert(ctx, BatchRunArgs{Action: action, BatchRunID: batchRunID}, &river.InsertOpts{Queue: queueName, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

func (q *Queue) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := q.client.Stop(ctx)
	q.pool.Close()
	return err
}
