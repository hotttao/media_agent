package model

import (
	"time"

	"gorm.io/datatypes"
)

// Job 对应 PG media_workflow.jobs 表。
type Job struct {
	ID                  string         `gorm:"column:id;primaryKey"`
	Scope               string         `gorm:"column:scope;not null"`
	WorkflowID          string         `gorm:"column:workflow_id;not null"`
	WorkflowVersion     int            `gorm:"column:workflow_version;not null"`
	Input               datatypes.JSON `gorm:"column:input;type:jsonb;not null"`
	Status              string         `gorm:"column:status;not null;default:pending"`
	RetryCount          int            `gorm:"column:retry_count;not null;default:0"`
	RetryHistory        datatypes.JSON `gorm:"column:retry_history;type:jsonb;not null;default:[]"`
	SubmittedBy         string         `gorm:"column:submitted_by;not null"`
	SubmittedAt         time.Time      `gorm:"column:submitted_at;not null"`
	StartedAt           *time.Time     `gorm:"column:started_at"`
	FinishedAt          *time.Time     `gorm:"column:finished_at"`
	FailureReason       string         `gorm:"column:failure_reason"`
	ConductorWorkflowID string         `gorm:"column:conductor_workflow_id"`
	CompilationHash     string         `gorm:"column:compilation_hash"`
}

func (Job) TableName() string { return "media_workflow.jobs" }

// StepRun 对应 media_workflow.step_runs 表。
type StepRun struct {
	ID              string         `gorm:"column:id;primaryKey"`
	JobID           string         `gorm:"column:job_id;not null"`
	StepKey         string         `gorm:"column:step_key;not null"`
	StepOrder       int            `gorm:"column:step_order;not null"`
	TemplateID      string         `gorm:"column:template_id;not null"`
	TemplateVersion int            `gorm:"column:template_version;not null"`
	ApprovalPolicy  string         `gorm:"column:approval_policy;not null;default:when_multiple_candidates"`
	Status          string         `gorm:"column:status;not null;default:blocked"`
	RetryCount      int            `gorm:"column:retry_count;not null;default:0"`
	ResolvedInputs  datatypes.JSON `gorm:"column:resolved_inputs;type:jsonb;not null;default:{}"`
	RenderedPrompt  string         `gorm:"column:rendered_prompt"`
	Candidates      datatypes.JSON `gorm:"column:candidates;type:jsonb;not null;default:[]"`
	SelectedIndex   *int           `gorm:"column:selected_index"`
	OutputAssetID   string         `gorm:"column:output_asset_id"`
	FailureReason   string         `gorm:"column:failure_reason"`
	ClaimedBy       string         `gorm:"column:claimed_by"`
	ClaimedAt       *time.Time     `gorm:"column:claimed_at"`
	StartedAt       *time.Time     `gorm:"column:started_at"`
	FinishedAt      *time.Time     `gorm:"column:finished_at"`
	// 反范式化（从 template 快照，ClaimStepRun 免 join）
	Platform       string `gorm:"column:platform;not null;default:''"`
	GenWay         string `gorm:"column:gen_way;not null;default:''"`
	GenType        string `gorm:"column:gen_type"`
	AssetKind      string `gorm:"column:asset_kind;not null;default:''"`
	NeedsConfirm   bool   `gorm:"column:needs_confirm;not null;default:false"`
	CandidateCount int    `gorm:"column:candidate_count;not null;default:1"`
}

func (StepRun) TableName() string { return "media_workflow.step_runs" }

type WorkerDispatch struct {
	ID                  string         `gorm:"column:id;primaryKey"`
	ConductorTaskID     string         `gorm:"column:conductor_task_id;not null"`
	ConductorWorkflowID string         `gorm:"column:conductor_workflow_id;not null"`
	JobID               string         `gorm:"column:job_id;not null"`
	StepRunID           string         `gorm:"column:step_run_id;not null"`
	Attempt             int            `gorm:"column:attempt;not null"`
	CapabilityKey       string         `gorm:"column:capability_key;not null"`
	CapabilityVersion   int            `gorm:"column:capability_version;not null"`
	UserID              string         `gorm:"column:user_id;not null"`
	OrganizationID      string         `gorm:"column:organization_id;not null"`
	CredentialID        string         `gorm:"column:credential_id;not null"`
	WorkerInstanceID    string         `gorm:"column:worker_instance_id;not null"`
	Status              string         `gorm:"column:status;not null"`
	InputJSON           datatypes.JSON `gorm:"column:input_json;type:jsonb;not null"`
	LeaseExpiresAt      time.Time      `gorm:"column:lease_expires_at;not null"`
	CreatedAt           time.Time      `gorm:"column:created_at;not null"`
	DeliveredAt         *time.Time     `gorm:"column:delivered_at"`
	CompletedAt         *time.Time     `gorm:"column:completed_at"`
}

func (WorkerDispatch) TableName() string { return "media_workflow.worker_dispatches" }

type WorkerResultInbox struct {
	ID              string         `gorm:"column:id;primaryKey"`
	DispatchID      string         `gorm:"column:dispatch_id;not null"`
	Attempt         int            `gorm:"column:attempt;not null"`
	Operation       string         `gorm:"column:operation;not null"`
	IdempotencyKey  string         `gorm:"column:idempotency_key;not null"`
	ResultJSON      datatypes.JSON `gorm:"column:result_json;type:jsonb;not null"`
	ResultDigest    string         `gorm:"column:result_digest;not null"`
	ConductorStatus string         `gorm:"column:conductor_status;not null"`
	LastError       string         `gorm:"column:last_error"`
	CreatedAt       time.Time      `gorm:"column:created_at;not null"`
	SubmittedAt     *time.Time     `gorm:"column:submitted_at"`
}

func (WorkerResultInbox) TableName() string { return "media_workflow.worker_result_inbox" }
