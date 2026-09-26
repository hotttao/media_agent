package job

import "context"

type JobRepo interface {
	Save(ctx context.Context, j *Job, stepRuns []*StepRun) error
	Get(ctx context.Context, id string) (*Job, error)
	GetWithStepRuns(ctx context.Context, id string) (*Job, []*StepRun, error)
	List(ctx context.Context, q ListQuery) ([]*Job, int, error)
	UpdateStatus(ctx context.Context, id string, status JobStatus, failureReason string) error
	UpdateInput(ctx context.Context, id string, input JobInput) error
	AppendRetryHistory(ctx context.Context, id string, record RetryRecord, retryCount int) error
	UpdateConductorExecution(ctx context.Context, id, conductorWorkflowID, compilationHash string) error
}

type WorkflowEngine interface {
	Start(ctx context.Context, workflowName string, workflowVersion int, input map[string]any) (string, error)
	Get(ctx context.Context, workflowID string) (*EngineExecution, error)
}

type EngineExecution struct {
	Status string
	Output map[string]any
	Tasks  []EngineTask
}

type EngineTask struct {
	TaskType          string
	ReferenceTaskName string
	Status            string
	Input             map[string]any
	Output            map[string]any
}

type StepRunRepo interface {
	Get(ctx context.Context, id string) (*StepRun, error)
	ListByJob(ctx context.Context, jobID string) ([]*StepRun, error)
	// ClaimAtomic 原子领取一个 PENDING StepRun（按 platform/gen_way 匹配），返回被领的 step_run + 其 template 信息。
	ClaimAtomic(ctx context.Context, q ClaimQuery) (*StepRun, error)
	UpdateStatus(ctx context.Context, id string, status StepRunStatus, updates map[string]any) error
	UpdateResolvedInputs(ctx context.Context, id string, inputs map[string]any, prompt string) error
	UpdateCandidates(ctx context.Context, id string, candidates []Candidate) error
	UpdateOutput(ctx context.Context, id string, outputAssetID string) error
	UpdateFailure(ctx context.Context, id string, reason string) error
	// ListClaimableByStepKeys 按 step_key 集合查 PENDING 的 step_run（DAG 推进用）。
	ListByJobAndStatus(ctx context.Context, jobID string, status StepRunStatus) ([]*StepRun, error)
	// ReclaimStale 把 CLAIMED 超时的 step_run 回 PENDING（worker 崩溃保护）。
	ReclaimStale(ctx context.Context, timeoutSeconds int) (int64, error)
}

type ListQuery struct {
	Scope      string
	WorkflowID string
	Status     string
	Limit      int
	Offset     int
}

type ClaimQuery struct {
	Platform string
	GenWay   string
	GenType  string
	WorkerID string
}
