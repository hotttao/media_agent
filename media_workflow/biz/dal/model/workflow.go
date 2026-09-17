package model

import "time"

// Workflow 对应 PG media_workflow.workflows 表。
type Workflow struct {
	ID            string     `gorm:"column:id;primaryKey"`
	Scope         string     `gorm:"column:scope;not null"`
	Name          string     `gorm:"column:name;not null"`
	Description   string     `gorm:"column:description"`
	OutputStepKey string     `gorm:"column:output_step_key;not null"`
	Status        string     `gorm:"column:status;not null;default:draft"`
	Visibility    string     `gorm:"column:visibility;not null;default:private"`
	Version       int        `gorm:"column:version;not null;default:1"`
	InputSchema   []byte     `gorm:"column:input_schema;type:jsonb;not null;default:[]"`
	CreatedBy     string     `gorm:"column:created_by;not null"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null"`
	DeletedAt     *time.Time `gorm:"column:deleted_at"`
}

func (Workflow) TableName() string { return "media_workflow.workflows" }

// WorkflowStep 对应 workflow_steps 表。
type WorkflowStep struct {
	ID              string    `gorm:"column:id;primaryKey"`
	WorkflowID      string    `gorm:"column:workflow_id;not null"`
	Version         int       `gorm:"column:version;not null"`
	StepKey         string    `gorm:"column:step_key;not null"`
	StepName        string    `gorm:"column:step_name"`
	StepOrder       int       `gorm:"column:step_order;not null"`
	TemplateID      string    `gorm:"column:template_id;not null"`
	TemplateVersion int       `gorm:"column:template_version;not null"`
	ApprovalPolicy  string    `gorm:"column:approval_policy;not null;default:when_multiple_candidates"`
	ReusePolicy     string    `gorm:"column:reuse_policy;not null;default:reuse_if_available"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
}

func (WorkflowStep) TableName() string { return "media_workflow.workflow_steps" }

// StepBinding 对应 step_bindings 表。
type StepBinding struct {
	ID           string    `gorm:"column:id;primaryKey"`
	StepID       string    `gorm:"column:step_id;not null"`
	ToInput      string    `gorm:"column:to_input;not null"`
	Source       string    `gorm:"column:source;not null"`
	Ref          *string   `gorm:"column:ref"`
	ConstValue   *string   `gorm:"column:const_value"`
	DisplayOrder int       `gorm:"column:display_order;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
}

func (StepBinding) TableName() string { return "media_workflow.step_bindings" }

// WorkflowGrant 对应 workflow_grants 表。
type WorkflowGrant struct {
	WorkflowID string    `gorm:"column:workflow_id;primaryKey"`
	TeamID     string    `gorm:"column:team_id;primaryKey"`
	Permission string    `gorm:"column:permission;not null"`
	GrantedBy  string    `gorm:"column:granted_by;not null"`
	GrantedAt  time.Time `gorm:"column:granted_at;not null"`
}

func (WorkflowGrant) TableName() string { return "media_workflow.workflow_grants" }
