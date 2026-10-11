package compiler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	capability "media_agent/media_workflow/biz/domain/capability"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/executable"
	"media_agent/media_workflow/biz/workflow/reuse_task"
)

func TestV4ReuseIsAnEngineBranchBeforeTheUnchangedGenerationLoop(t *testing.T) {
	step := executable.ExecutableStepIR{Key: "image", Template: executable.TemplateIR{ID: "tpl", Version: 1, Body: "dress"}, Capability: executable.CapabilityIR{Key: "image", Version: 1, ConductorTaskDef: "image_v1", Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}}}, Invocation: template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}}, CandidateCount: 1, ApprovalPolicy: "always", ReusePolicy: "reuse_if_available", OutputAssetKind: "model_image"}
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: "wf", WorkflowVersion: 1, OutputStepKey: step.Key, Steps: []executable.ExecutableStepIR{step}}
	d, err := CompileExecutableIR(ir)
	require.NoError(t, err)
	before, err := DefinitionHash(d)
	require.NoError(t, err)
	frozen, err := json.Marshal(d)
	require.NoError(t, err)
	var restored Definition
	require.NoError(t, json.Unmarshal(frozen, &restored))
	after, err := DefinitionHash(restored)
	require.NoError(t, err)
	require.Equal(t, before, after, "publication must verify the same immutable hash after JSON/JSONB replay")
	require.Len(t, d.Tasks, 4)
	require.Equal(t, taskref.Reference(step.Key, "prepare"), d.Tasks[0].TaskReferenceName)
	require.Equal(t, reuse_task.TaskDefinitionName, d.Tasks[1].Name)
	require.Equal(t, "SWITCH", d.Tasks[2].Type)
	require.Empty(t, d.Tasks[2].DecisionCases["reuse"], "cache hit cannot schedule a capability or human review")
	loop := d.Tasks[2].DecisionCases["generate"][0]
	require.Equal(t, "DO_WHILE", loop.Type)
	require.Equal(t, "image_v1", loop.LoopOver[1].Name)
	require.Equal(t, "HUMAN", loop.LoopOver[2].Type)
	require.Equal(t, finalizeRef(step.Key), d.Tasks[3].TaskReferenceName)
	require.Equal(t, "${"+taskref.Reference(step.Key, "reuse")+".output}", d.Tasks[3].InputParameters["reuse_result"])
	ir.SchemaVersion = executable.LegacySchemaVersion
	legacy, err := CompileExecutableIR(ir)
	require.NoError(t, err)
	require.Len(t, legacy.Tasks, 2)
	require.Equal(t, "DO_WHILE", legacy.Tasks[0].Type, "frozen legacy execution semantics must not gain reuse")
}
