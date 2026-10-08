package template

import "time"

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
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

// Slot 模板输入槽位。
// Name: prompt_body 占位符；MATERIAL/ASSET 时 = asset_kind（类型）。
// ResourceType: MATERIAL/ASSET/PRODUCT/VIRTUAL_IP/TEXT。
type Slot struct {
	Name         string
	AssetKind    string
	ResourceType string
	Required     bool
	DisplayOrder int
}

// InvocationOp is the closed expression set accepted by invocation mappings.
// Keeping this set closed makes published templates deterministic and safe to
// evaluate without arbitrary scripts.
type InvocationOp string

const (
	InvocationRenderTemplate InvocationOp = "render_template"
	InvocationAssetRef       InvocationOp = "asset_ref"
	InvocationList           InvocationOp = "list"
)

// InvocationExpression maps business slots to one capability input. Slot is
// used by asset_ref and Items is used by list.
type InvocationExpression struct {
	Op    InvocationOp           `json:"op"`
	Slot  string                 `json:"slot,omitempty"`
	Items []InvocationExpression `json:"items,omitempty"`
}

type InvocationMapping map[string]InvocationExpression

// Template 提示词模板（当前版本）。版本化：Update 时 version+1，旧版本入 TemplateVersion。
type Template struct {
	OrganizationID    string
	ID                string
	Scope             string
	Name              string
	Description       string
	PromptBody        string
	Inputs            []Slot
	OutputAssetKind   string // → asset_kinds.name，resource_type=ASSET
	Platform          string
	GenWay            string
	GenType           string
	NeedsConfirm      bool
	CandidateCount    int
	CapabilityKey     string
	CapabilityVersion int
	InvocationMapping InvocationMapping
	Status            Status
	Visibility        Visibility
	Version           int
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// TemplateVersion 历史版本快照（Update 时追加）。
type TemplateVersion struct {
	Status            Status
	TemplateID        string
	Version           int
	PromptBody        string
	OutputAssetKind   string
	Platform          string
	GenWay            string
	GenType           string
	NeedsConfirm      bool
	CandidateCount    int
	CapabilityKey     string
	CapabilityVersion int
	InvocationMapping InvocationMapping
	Inputs            []Slot
	CreatedBy         string
	CreatedAt         time.Time
}

// Grant template ↔ team 授权。
type Grant struct {
	TemplateID string
	TeamID     string
	Permission Permission
	GrantedBy  string
	GrantedAt  time.Time
}
