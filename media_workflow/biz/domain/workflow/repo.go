package workflow

import (
	"context"
	"time"
)

type Repo interface {
	// Save 创建 workflow（含 steps + bindings + input_schema）。version=1。
	Save(ctx context.Context, w *Workflow) error
	// Get 取（含 steps + bindings）。
	Get(ctx context.Context, id string) (*Workflow, error)
	// List 分页。
	List(ctx context.Context, q ListQuery) ([]*WorkflowSummary, int, error)
	// Update 整体覆盖（删旧 steps+bindings 重建，更新主表 + input_schema）；version+1。
	Update(ctx context.Context, w *Workflow) error
	// UpdateStatus 改 status。
	UpdateStatus(ctx context.Context, id string, status Status) error
	// UpdateVisibility 改 visibility。
	UpdateVisibility(ctx context.Context, id string, visibility Visibility) error
	// Delete 软删。
	Delete(ctx context.Context, id string) error
}

type GrantRepo interface {
	AddGrant(ctx context.Context, g *Grant) error
	RemoveGrant(ctx context.Context, workflowID, teamID string) error
	ListGrants(ctx context.Context, workflowID string) ([]*Grant, error)
}

// WorkflowSummary 列表项（不含 steps/bindings，按需 Get）。
type WorkflowSummary struct {
	OrganizationID string
	WorkspaceID    string
	ID             string
	Scope          string
	Name           string
	Description    string
	OutputStepKey  string
	Status         Status
	Visibility     Visibility
	Version        int
	InputSchema    []InputSchemaEntry
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ListQuery struct {
	OrganizationIDs []string
	WorkspaceIDs    []string
	EnforceBoundary bool
	EnforceAccess   bool
	CallerID        string
	TeamIDs         []string
	Scope           string
	Name            string
	Status          string
	Visibility      string
	Limit           int
	Offset          int
}
