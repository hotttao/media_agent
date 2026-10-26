// Package execution defines worker execution facts and ports. Scheduling and leases are owned by the gateway.
package execution

import (
	"context"
	"errors"
)

type Capability struct {
	Key         string
	Version     int
	Concurrency int
}
type Task struct {
	DispatchID        string
	Attempt           int
	CapabilityKey     string
	CapabilityVersion int
	Input             map[string]any
	LeaseExpiresAt    int64
	TraceParent       string
}
type ResultReceipt struct {
	ReceiptID  string
	DispatchID string
	Status     string
}
type Result struct {
	Task        Task
	Key         string
	Output      map[string]any
	Reason      string
	FailureKind string
}

var ErrResultRejected = errors.New("worker result rejected")

type Executor interface {
	AvailableCapabilities() []Capability
	ExecuteContext(context.Context, Task) (map[string]any, error)
}
type Gateway interface {
	Register(context.Context, string, []Capability) error
	Poll(context.Context, string, Capability) (*Task, error)
	Heartbeat(context.Context, string, Task) error
	Complete(context.Context, string, Task, string, map[string]any) (*ResultReceipt, error)
	FailWithKind(context.Context, string, Task, string, string, string) (*ResultReceipt, error)
}
type Uploader interface {
	UploadFile(context.Context, string, Task, int, string) (string, error)
}
type ResultRepository interface {
	Save(Result) error
	Pending() ([]Result, error)
	Finish(Result, bool) error
}

// Checkpoint holds opaque generation references without coupling execution to a provider domain.
// It records facts only; it never grants execution eligibility or schedules work.
type GenerationIntent struct {
	Prompt          string
	Images          []Artifact
	Model           string
	DurationSeconds int
	AspectRatio     string
}
type Checkpoint struct {
	Task          Task
	Intent        GenerationIntent
	Closed        bool
	GenerationKey string
	RequestDigest string
	Reference     string
	Artifacts     []Artifact
}
type Artifact struct {
	ChecksumSHA256 string
	SizeBytes      int64
	ID             string
	LocalPath      string
	ObjectPath     string
}
type CheckpointRepository interface {
	Save(context.Context, Checkpoint) error
	Load(context.Context, string, int) (*Checkpoint, error)
	Pending(context.Context) ([]Checkpoint, error)
}

// ErrExecutionPending keeps the same attempt and local slot while a side effect
// is uncertain or requires human action. It must never become a failed receipt.
var ErrExecutionPending = errors.New("execution pending; retain original attempt")
var ErrExecutionAbandoned = errors.New("execution is no longer authorized")

type Admission interface {
	Acquire(context.Context) (func(), error)
}
type ExecutionRecovery interface {
	PendingTasks(context.Context) ([]Task, error)
	FinishTask(context.Context, Task) error
}

type InputDownloader interface {
	Fetch(context.Context, string, string) (Artifact, error)
}

type RecoveryAdmission interface {
	AcquireRecovery(context.Context) (func(), error)
}

type ArtifactVerifier interface {
	Verify(context.Context, Artifact) error
}
