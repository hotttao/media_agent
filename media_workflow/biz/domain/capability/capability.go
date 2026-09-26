package capability

import (
	"context"
	"time"
)

type InputType string

const (
	InputString          InputType = "string"
	InputResourceRef     InputType = "resource_ref"
	InputResourceRefList InputType = "resource_ref_list"
)

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
