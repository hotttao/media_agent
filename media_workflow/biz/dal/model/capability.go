package model

import "time"

type ExecutionCapability struct {
	CapabilityKey    string     `gorm:"column:capability_key;primaryKey"`
	Version          int        `gorm:"column:version;primaryKey"`
	Provider         string     `gorm:"column:provider;not null"`
	Mode             string     `gorm:"column:mode;not null"`
	InputSchema      []byte     `gorm:"column:input_schema;type:jsonb;not null"`
	OutputAssetKind  string     `gorm:"column:output_asset_kind;not null"`
	ConductorTaskDef string     `gorm:"column:conductor_task_def;not null"`
	ExecutionTarget  string     `gorm:"column:execution_target;not null"`
	ManagedPoolKey   *string    `gorm:"column:managed_pool_key"`
	RetryPolicy      []byte     `gorm:"column:retry_policy;type:jsonb;not null"`
	TimeoutSeconds   int        `gorm:"column:timeout_seconds;not null"`
	Status           string     `gorm:"column:status;not null"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null"`
	PublishedAt      *time.Time `gorm:"column:published_at"`
}

func (ExecutionCapability) TableName() string {
	return "media_workflow.execution_capabilities"
}

type WorkflowCompilation struct {
	ID               string     `gorm:"column:id;primaryKey"`
	WorkflowID       string     `gorm:"column:workflow_id;not null"`
	WorkflowVersion  int        `gorm:"column:workflow_version;not null"`
	ConductorName    string     `gorm:"column:conductor_name;not null"`
	ConductorVersion int        `gorm:"column:conductor_version;not null"`
	IRJSON           []byte     `gorm:"column:ir_json;type:jsonb;not null"`
	DefinitionJSON   []byte     `gorm:"column:definition_json;type:jsonb;not null"`
	DefinitionHash   string     `gorm:"column:definition_hash;not null"`
	Status           string     `gorm:"column:status;not null"`
	ErrorMessage     string     `gorm:"column:error_message"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null"`
	RegisteredAt     *time.Time `gorm:"column:registered_at"`
}

func (WorkflowCompilation) TableName() string {
	return "media_workflow.workflow_compilations"
}
