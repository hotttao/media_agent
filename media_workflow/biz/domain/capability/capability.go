package capability

import (
	"context"
	"errors"
	"time"
)

var ErrCapabilityNotFound = errors.New("capability not found")

type InputType string

type ExecutionTarget string

// MediaType describes Provider data, not a Template's business Asset Kind.
type MediaType string

const (
	MediaImage MediaType = "IMAGE"
	MediaVideo MediaType = "VIDEO"
	MediaText  MediaType = "TEXT"
)

func (m MediaType) Valid() bool { return m == MediaImage || m == MediaVideo || m == MediaText }

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
	Name      string    `json:"name"`
	Type      InputType `json:"type"`
	Required  bool      `json:"required"`
	MediaType MediaType `json:"media_type,omitempty"`
	MinItems  int       `json:"min_items,omitempty"`
	MaxItems  int       `json:"max_items,omitempty"`
}

func (i Input) ValidMediaConstraint() bool {
	if i.MinItems < 0 || i.MaxItems < 0 || (i.MaxItems > 0 && i.MinItems > i.MaxItems) || (i.Type != InputResourceRefList && (i.MinItems != 0 || i.MaxItems != 0)) {
		return false
	}
	return i.MediaType == "" || (i.MediaType.Valid() && (i.Type == InputResourceRef || i.Type == InputResourceRefList))
}

func (i Input) AcceptsResourceCount(count int) bool {
	return i.Type == InputResourceRefList && i.ValidMediaConstraint() && count >= 0 && count >= i.MinItems && (i.MaxItems == 0 || count <= i.MaxItems)
}

type Capability struct {
	Key               string
	Version           int
	Provider          string
	Mode              string
	GenerationMethod  GenerationMethod
	GenerationType    GenerationType
	Inputs            []Input
	OutputAssetKind   string
	OutputMediaType   MediaType
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
