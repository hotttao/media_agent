package job

import "context"

type JobRepo interface {
	Save(ctx context.Context, j *Job, stepRuns []*StepRun) error
	SaveImported(ctx context.Context, j *Job, stepRuns []*StepRun) error
	GetByLegacySource(ctx context.Context, workspaceID, sourceSystem, sourceRecordID string) (*Job, error)
	Get(ctx context.Context, id string) (*Job, error)
	GetWithStepRuns(ctx context.Context, id string) (*Job, []*StepRun, error)
	GetByConductorWorkflowID(ctx context.Context, conductorWorkflowID string) (*Job, error)
	List(ctx context.Context, q ListQuery) ([]*Job, int, error)
	UpdateStatus(ctx context.Context, id string, status JobStatus, failureReason string) error
	UpdateInput(ctx context.Context, id string, input JobInput) error
	AppendRetryHistory(ctx context.Context, id string, record RetryRecord, retryCount int) error
	UpdateConductorExecution(ctx context.Context, id, conductorWorkflowID, compilationHash string) error
}

type WorkflowEngine interface {
	Start(ctx context.Context, workflowName string, workflowVersion int, correlationID string, input map[string]any, taskToDomain map[string]string) (string, error)
	Get(ctx context.Context, workflowID string) (*EngineExecution, error)
	CompleteTask(ctx context.Context, workflowID, taskReferenceName string, output map[string]any) error
	RerunFromTask(ctx context.Context, workflowID, taskID string) error
	RerunWorkflow(ctx context.Context, workflowID string, input map[string]any) error
	Terminate(ctx context.Context, workflowID, reason string) error
}

type CapabilityTaskEngine interface {
	PollCapabilityTask(ctx context.Context, taskType, workerID, executionDomain string) (*CapabilityTask, error)
	UpdateCapabilityTask(ctx context.Context, result CapabilityTaskResult) error
}

// FailureRetryEngine continues unsuccessful tasks using the execution engine's
// own semantics; it does not reset business steps or change workflow input.
type FailureRetryEngine interface {
	RetryWorkflow(context.Context, string) error
}

// ApprovalTaskEngine submits to an immutable engine task identity. Reference
// names can be reused during Retry and must never redirect a durable decision.
type ApprovalTaskEngine interface {
	CompleteApprovalTask(ctx context.Context, workflowID, taskID string, output map[string]any) error
}

type CapabilityTask struct {
	TaskType, ReferenceTaskName, TaskID, WorkflowInstanceID string
	RetryCount                                              int
	Input                                                   map[string]any
}

type CapabilityTaskResult struct {
	WorkflowInstanceID, TaskID, WorkerID, Status string
	Output                                       map[string]any
	Reason                                       string
	CallbackAfterSeconds                         int
}

type EngineExecution struct {
	WorkflowInstanceID string
	DefinitionName     string
	DefinitionVersion  int
	Definition         map[string]any
	Input              map[string]any
	TaskDomains        map[string]string
	Status             string
	Output             map[string]any
	Tasks              []EngineTask
}

type EngineTask struct {
	Sequence                          *int
	TaskID                            string
	TaskType                          string
	ReferenceTaskName                 string
	Status                            string
	Input                             map[string]any
	Output                            map[string]any
	ScheduledTime, StartTime, EndTime int64
	RetryCount                        int
	Retried                           bool
	ReasonForIncompletion             string
}

type StepRunRepo interface {
	Get(ctx context.Context, id string) (*StepRun, error)
	ListByJob(ctx context.Context, jobID string) ([]*StepRun, error)
	GetByJobAndStepKey(ctx context.Context, jobID, stepKey string) (*StepRun, error)
	UpdateStatus(ctx context.Context, id string, status StepRunStatus, updates map[string]any) error
	UpdateResolvedInputs(ctx context.Context, id string, inputs map[string]any, prompt string) error
	UpdateCandidates(ctx context.Context, id string, candidates []Candidate) error
	UpdateOutput(ctx context.Context, id string, outputAssetID string) error
	UpdateFailure(ctx context.Context, id string, reason string) error
}

type ListQuery struct {
	OrganizationID string
	WorkspaceID    string
	Scope          string
	WorkflowID     string
	SubmittedBy    string
	Status         string
	Limit          int
	Offset         int
}
