package compiler

import (
	"testing"

	"github.com/stretchr/testify/require"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/workflow/executable"
)

func TestCompileExecutableIRInlinesRendererAndCapabilityMapping(t *testing.T) {
	ir := executable.ExecutableWorkflowIR{
		SchemaVersion: executable.SchemaVersion, WorkflowID: "clothing-video", WorkflowVersion: 2, OutputStepKey: "model_image",
		Steps: []executable.ExecutableStepIR{{
			Key: "model_image", Order: 1,
			Template: executable.TemplateIR{ID: "tpl_model", Version: 3, Body: "{{.person.description}} wears {{.garment.name}}"},
			Context: map[string]executable.ValueRef{
				"person":  {Source: executable.ValueWorkflowInput, Ref: "full_body"},
				"garment": {Source: executable.ValueWorkflowInput, Ref: "clothing"},
			},
			Capability: executable.CapabilityIR{Key: "jimeng.image.multi", Version: 1, ConductorTaskDef: "jimeng_image_multi_v1"},
			Invocation: template.InvocationMapping{
				"prompt": {Op: template.InvocationRenderTemplate},
				"images": {Op: template.InvocationList, Items: []template.InvocationExpression{
					{Op: template.InvocationAssetRef, Slot: "person"},
					{Op: template.InvocationAssetRef, Slot: "garment"},
				}},
			},
			OutputAssetKind: "model_image", ApprovalPolicy: "when_multiple_candidates", ReusePolicy: "reuse_if_available",
		}},
	}

	definition, err := CompileExecutableIR(ir)
	require.NoError(t, err)
	require.Len(t, definition.Tasks, 3)
	require.Equal(t, RenderTemplateTaskName, definition.Tasks[0].Name)
	require.Equal(t, "jimeng_image_multi_v1", definition.Tasks[1].Name)
	require.Equal(t, "${step__model_image__render.output.prompt}", definition.Tasks[1].InputParameters["prompt"])
	require.Equal(t, []any{
		"${workflow.input.inputs.full_body.asset_ref}",
		"${workflow.input.inputs.clothing.asset_ref}",
	}, definition.Tasks[1].InputParameters["images"])
	require.Equal(t, "${step__model_image__finalize.output.output_resource_id}", definition.OutputParameters["output_resource_id"])
}

func TestCompileExecutableIRUsesStaticForkForParallelSteps(t *testing.T) {
	ir := executable.ExecutableWorkflowIR{
		SchemaVersion: executable.SchemaVersion, WorkflowID: "parallel", WorkflowVersion: 1, OutputStepKey: "finish",
		Steps: []executable.ExecutableStepIR{
			minimalIRStep("a", 1, nil),
			minimalIRStep("b", 2, nil),
			minimalIRStep("finish", 3, []string{"a", "b"}),
		},
	}
	definition, err := CompileExecutableIR(ir)
	require.NoError(t, err)
	require.Equal(t, "FORK_JOIN", definition.Tasks[0].Type)
	require.Len(t, definition.Tasks[0].ForkTasks, 2)
	require.Equal(t, []string{"step__a__finalize", "step__b__finalize"}, definition.Tasks[1].JoinOn)
	require.Equal(t, RenderTemplateTaskName, definition.Tasks[2].Name)
}

func TestCompileExecutableIRInsertsHumanApprovalBeforeFinalize(t *testing.T) {
	step := minimalIRStep("image", 1, nil)
	step.ApprovalPolicy = "always"
	step.ApprovalRequired = true
	step.CandidateCount = 1
	ir := executable.ExecutableWorkflowIR{
		SchemaVersion: executable.SchemaVersion, WorkflowID: "approval", WorkflowVersion: 1,
		OutputStepKey: "image", Steps: []executable.ExecutableStepIR{step},
	}

	definition, err := CompileExecutableIR(ir)
	require.NoError(t, err)
	require.Len(t, definition.Tasks, 4)
	require.Equal(t, "HUMAN", definition.Tasks[2].Type)
	require.Equal(t, "step__image__approval", definition.Tasks[2].TaskReferenceName)
	require.Equal(t, "${step__image__approval.output.selected_candidate}", definition.Tasks[3].InputParameters["selected_candidate"])
}

func minimalIRStep(key string, order int, dependencies []string) executable.ExecutableStepIR {
	return executable.ExecutableStepIR{
		Key: key, Order: order, DependsOn: dependencies,
		Template:        executable.TemplateIR{ID: "tpl_" + key, Version: 1, Body: "prompt"},
		Context:         map[string]executable.ValueRef{},
		Capability:      executable.CapabilityIR{Key: "cap", Version: 1, ConductorTaskDef: "cap_v1"},
		Invocation:      template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}},
		OutputAssetKind: "image",
	}
}
