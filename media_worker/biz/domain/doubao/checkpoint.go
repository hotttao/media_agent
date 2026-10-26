package doubao

import (
	"context"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

// Checkpoint holds the dedicated Studio generation facts for one Worker attempt.
// It records facts only; it never grants execution eligibility or schedules work.
type GenerationIntent struct {
	Prompt          string
	Images          []worker_runtime.Artifact
	Model           string
	DurationSeconds int
	AspectRatio     string
}
type Checkpoint struct {
	Task          worker_runtime.Task
	Intent        GenerationIntent
	Closed        bool
	GenerationKey string
	RequestDigest string
	Reference     string
	Artifacts     []worker_runtime.Artifact
}
type CheckpointRepository interface {
	Save(context.Context, Checkpoint) error
	Load(context.Context, string, int) (*Checkpoint, error)
	Pending(context.Context) ([]Checkpoint, error)
}
