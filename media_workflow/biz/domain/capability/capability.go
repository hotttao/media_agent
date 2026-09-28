package capability

import (
	"context"
	"time"
)

type InputType string

type ExecutionTarget string

const (
	InputString          InputType = "string"
	InputResourceRef     InputType = "resource_ref"
	InputResourceRefList InputType = "resource_ref_list"
)

const (
	ExecutionTargetPersonalWorker ExecutionTarget = "personal_worker"
	ExecutionTargetManagedWorker  ExecutionTarget = "managed_worker"
)

func (t ExecutionTarget) Valid() bool {
	return t == ExecutionTargetPersonalWorker || t == ExecutionTargetManagedWorker
}

type Input struct {
	Name     string    `json:"name"`
	Type     InputType `json:"type"`
	Required bool      `json:"required"`
}

type Capability struct {
	Key               string
	Version           int
	Provider          string
	Mode              string
	Inputs            []Input
	OutputAssetKind   string
	ConductorTaskDef  string
	ExecutionTarget   ExecutionTarget
	ManagedPoolKey    string
	RetryCount        int
	RetryLogic        string
	RetryDelaySeconds int
	TimeoutSeconds    int
	Status            string
	CreatedAt         time.Time
	PublishedAt       *time.Time
}

type Repo interface {
	Get(ctx context.Context, key string, version int) (*Capability, error)
	ListPublished(ctx context.Context) ([]*Capability, error)
}
