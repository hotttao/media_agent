// Package executable defines the stable, engine-neutral boundary between the
// semantic compiler and execution-engine backends.
package executable

import template "media_agent/media_workflow/biz/domain/template"

const SchemaVersion = 1

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
	Key              string `json:"key"`
	Version          int    `json:"version"`
	ConductorTaskDef string `json:"conductor_task_def"`
}

type ExecutableStepIR struct {
	Key             string                     `json:"key"`
	Order           int                        `json:"order"`
	DependsOn       []string                   `json:"depends_on,omitempty"`
	Template        TemplateIR                 `json:"template"`
	Context         map[string]ValueRef        `json:"context"`
	Capability      CapabilityIR               `json:"capability"`
	Invocation      template.InvocationMapping `json:"invocation"`
	OutputAssetKind string                     `json:"output_asset_kind"`
	ApprovalPolicy  string                     `json:"approval_policy"`
	ReusePolicy     string                     `json:"reuse_policy"`
}

type ExecutableWorkflowIR struct {
	SchemaVersion   int                `json:"schema_version"`
	WorkflowID      string             `json:"workflow_id"`
	WorkflowVersion int                `json:"workflow_version"`
	OutputStepKey   string             `json:"output_step_key"`
	Steps           []ExecutableStepIR `json:"steps"`
}
