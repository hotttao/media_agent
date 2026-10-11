// Package executable defines the stable, engine-neutral boundary between the
// semantic compiler and execution-engine backends.
package executable

import (
	capability "media_agent/media_workflow/biz/domain/capability"
	template "media_agent/media_workflow/biz/domain/template"
)

const SchemaVersion = 4
const LegacySchemaVersion = 3

type ValueSource string

const (
	ValueWorkflowInput ValueSource = "workflow_input"
	ValueStepOutput    ValueSource = "step_output"
	ValueConst         ValueSource = "const"
)

// ValueRef resolves one business slot at runtime. ConstValue is populated only
// for ValueConst; Ref names a workflow input or predecessor step otherwise.
type ValueRef struct {
	Source     ValueSource `json:"source"`
	Ref        string      `json:"ref,omitempty"`
	ConstValue string      `json:"const_value,omitempty"`
}

type TemplateIR struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Body    string `json:"body"`
}

type CapabilityIR struct {
	Key              string             `json:"key"`
	Version          int                `json:"version"`
	ConductorTaskDef string             `json:"conductor_task_def"`
	ExecutionTarget  string             `json:"execution_target"`
	ManagedPoolKey   string             `json:"managed_pool_key,omitempty"`
	Provider         string             `json:"provider,omitempty"`
	GenerationMethod string             `json:"generation_method,omitempty"`
	GenerationType   string             `json:"generation_type,omitempty"`
	OutputMediaType  string             `json:"output_media_type,omitempty"`
	Inputs           []capability.Input `json:"inputs,omitempty"`
}

type ExecutableStepIR struct {
	Key              string                     `json:"key"`
	Order            int                        `json:"order"`
	DependsOn        []string                   `json:"depends_on,omitempty"`
	Template         TemplateIR                 `json:"template"`
	Context          map[string]ValueRef        `json:"context"`
	Capability       CapabilityIR               `json:"capability"`
	Invocation       template.InvocationMapping `json:"invocation"`
	OutputAssetKind  string                     `json:"output_asset_kind"`
	ApprovalPolicy   string                     `json:"approval_policy"`
	ApprovalRequired bool                       `json:"approval_required"`
	CandidateCount   int                        `json:"candidate_count"`
	ReusePolicy      string                     `json:"reuse_policy"`
}

type ExecutableWorkflowIR struct {
	SchemaVersion   int                `json:"schema_version"`
	WorkflowID      string             `json:"workflow_id"`
	WorkflowVersion int                `json:"workflow_version"`
	OutputStepKey   string             `json:"output_step_key"`
	Steps           []ExecutableStepIR `json:"steps"`
}
