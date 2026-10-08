package workflow

import "time"

type Status string

const (
	StatusDraft         Status = "draft"
	StatusPublishing    Status = "publishing"
	StatusPublishFailed Status = "publish_failed"
	StatusPublished     Status = "published"
	StatusArchived      Status = "archived"
)

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityTeam    Visibility = "team"
	VisibilityPublic  Visibility = "public"
)

type Permission string

const (
	PermissionViewer Permission = "viewer"
	PermissionEditor Permission = "editor"
)

type ApprovalPolicy string

const (
	ApprovalWhenMultipleCandidates ApprovalPolicy = "when_multiple_candidates"
	ApprovalAlways                 ApprovalPolicy = "always"
)

type ReusePolicy string

const (
	ReuseIfAvailable ReusePolicy = "reuse_if_available"
	ForceNew         ReusePolicy = "force_new"
)

// SlotBindingSource step 输入来源。
type SlotBindingSource string

const (
	SourceWorkflowInput SlotBindingSource = "WORKFLOW_INPUT"
	SourceStepOutput    SlotBindingSource = "STEP_OUTPUT"
	SourceConst         SlotBindingSource = "CONST"
)

// SlotBinding step 输入绑定（source 三选一，CHECK 互斥）。
type SlotBinding struct {
	ToInput      string // slot.name
	Source       SlotBindingSource
	Ref          string // WORKFLOW_INPUT → input name; STEP_OUTPUT → step.key
	ConstValue   string // CONST 字面值
	DisplayOrder int
}

// Step DAG 节点。
type Step struct {
	Key             string
	Name            string
	Order           int
	TemplateID      string
	TemplateVersion int
	ApprovalPolicy  ApprovalPolicy
	ReusePolicy     ReusePolicy
	SlotBindings    []SlotBinding
}

// InputSchemaEntry 派生的 workflow 输入契约项。
type InputSchemaEntry struct {
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
	Required     bool   `json:"required"`
	AssetKind    string `json:"asset_kind,omitempty"`
}

// Workflow 工作流（DAG）。input_schema 从 WORKFLOW_INPUT bindings 派生快照。
type Workflow struct {
	OrganizationID string
	WorkspaceID    string
	ID             string
	Scope          string
	Name           string
	Description    string
	OutputStepKey  string
	Steps          []Step
	Status         Status
	Visibility     Visibility
	Version        int
	InputSchema    []InputSchemaEntry
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// Read-only projection of the current version's publication receipt.
	PublicationID     string
	PublicationStatus string
}

// Grant workflow ↔ team 授权。
type Grant struct {
	WorkflowID string
	TeamID     string
	Permission Permission
	GrantedBy  string
	GrantedAt  time.Time
}
