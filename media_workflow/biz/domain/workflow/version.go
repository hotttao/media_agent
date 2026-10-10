package workflow

import (
	"context"
	"errors"
)

var ErrVersionNotFound = errors.New("workflow version not found")
var ErrVersionConflict = errors.New("workflow version snapshot conflicts")

// DefinitionVersion excludes mutable ownership, access and engine receipts.
type DefinitionVersion struct {
	Definition *Workflow
	Source     string // snapshot | compiled (only provable historical fields)
	Registered bool
}

type VersionRepo interface {
	GetDefinitionVersion(context.Context, string, int) (*DefinitionVersion, error)
}
