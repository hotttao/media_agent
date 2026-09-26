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
