package model

import "time"

// Template 对应 PG media_workflow.templates 表（当前版本）。
type Template struct {
	OrganizationID    string     `gorm:"column:organization_id;not null;default:''"`
	ID                string     `gorm:"column:id;primaryKey"`
	Scope             string     `gorm:"column:scope;not null"`
	Name              string     `gorm:"column:name;not null"`
	Description       string     `gorm:"column:description"`
	PromptBody        string     `gorm:"column:prompt_body;not null"`
	OutputAssetKind   string     `gorm:"column:output_asset_kind;not null"`
	Platform          string     `gorm:"column:platform;not null"`
	GenWay            string     `gorm:"column:gen_way;not null"`
	GenType           string     `gorm:"column:gen_type"`
	NeedsConfirm      bool       `gorm:"column:needs_confirm;not null;default:false"`
	CandidateCount    int        `gorm:"column:candidate_count;not null;default:1"`
	CapabilityKey     string     `gorm:"column:capability_key;not null"`
	CapabilityVersion int        `gorm:"column:capability_version;not null"`
	InvocationMapping []byte     `gorm:"column:invocation_mapping;type:jsonb;not null"`
	Status            string     `gorm:"column:status;not null;default:draft"`
	Visibility        string     `gorm:"column:visibility;not null;default:private"`
	Version           int        `gorm:"column:version;not null;default:1"`
	CreatedBy         string     `gorm:"column:created_by;not null"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt         *time.Time `gorm:"column:deleted_at"`
}

func (Template) TableName() string { return "media_workflow.templates" }

// TemplateInput 对应 template_inputs 表（当前版本的 input slots）。
type TemplateInput struct {
	ID           string    `gorm:"column:id;primaryKey"`
	TemplateID   string    `gorm:"column:template_id;not null"`
	Version      int       `gorm:"column:version;not null"`
	Name         string    `gorm:"column:name;not null"`
	AssetKind    string    `gorm:"column:asset_kind;not null"`
	ResourceType string    `gorm:"column:resource_type;not null"`
	Required     bool      `gorm:"column:required;not null;default:false"`
	DisplayOrder int       `gorm:"column:display_order;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
}

func (TemplateInput) TableName() string { return "media_workflow.template_inputs" }

// TemplateVersion 对应 template_versions 表（历史版本快照）。
type TemplateVersion struct {
	Status            string    `gorm:"column:status;not null;default:draft"`
	ID                string    `gorm:"column:id;primaryKey"`
	TemplateID        string    `gorm:"column:template_id;not null"`
	Version           int       `gorm:"column:version;not null"`
	PromptBody        string    `gorm:"column:prompt_body;not null"`
	OutputAssetKind   string    `gorm:"column:output_asset_kind;not null"`
	Platform          string    `gorm:"column:platform;not null"`
	GenWay            string    `gorm:"column:gen_way;not null"`
	GenType           string    `gorm:"column:gen_type"`
	NeedsConfirm      bool      `gorm:"column:needs_confirm;not null;default:false"`
	CandidateCount    int       `gorm:"column:candidate_count;not null;default:1"`
	CapabilityKey     string    `gorm:"column:capability_key;not null"`
	CapabilityVersion int       `gorm:"column:capability_version;not null"`
	InvocationMapping []byte    `gorm:"column:invocation_mapping;type:jsonb;not null"`
	InputsSnapshot    []byte    `gorm:"column:inputs_snapshot;type:jsonb;not null"`
	CreatedBy         string    `gorm:"column:created_by;not null"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
}

func (TemplateVersion) TableName() string { return "media_workflow.template_versions" }

// TemplateGrant 对应 template_grants 表。
type TemplateGrant struct {
	TemplateID string    `gorm:"column:template_id;primaryKey"`
	TeamID     string    `gorm:"column:team_id;primaryKey"`
	Permission string    `gorm:"column:permission;not null"`
	GrantedBy  string    `gorm:"column:granted_by;not null"`
	GrantedAt  time.Time `gorm:"column:granted_at;not null"`
}

func (TemplateGrant) TableName() string { return "media_workflow.template_grants" }
