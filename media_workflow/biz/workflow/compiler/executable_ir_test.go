package compiler

import (
	"testing"

	"github.com/stretchr/testify/require"
	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared/taskref"
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
			Capability: executable.CapabilityIR{Key: "jimeng.image.multi", Version: 1, ConductorTaskDef: "jimeng_image_multi_v1", ExecutionTarget: "personal_worker"},
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
	require.Len(t, definition.Tasks, 2)
	require.Equal(t, "DO_WHILE", definition.Tasks[0].Type)
	tasks := definition.Tasks[0].LoopOver
	require.Len(t, tasks, 3)
	require.Equal(t, RenderTemplateTaskName, tasks[0].Name)
	require.Equal(t, "last_regeneration_v1", tasks[0].InputParameters["feedback_mode"])
	require.Equal(t, "${"+taskref.Reference("model_image", "loop")+".output.iteration}", tasks[0].InputParameters["generation"])
	require.Equal(t, "${"+taskref.Reference("model_image", "approval")+".iteration}", tasks[0].InputParameters["previous_generation"])
	require.Equal(t, "${"+taskref.Reference("model_image", "approval")+".output}", tasks[0].InputParameters["previous_review"])
	require.Equal(t, "jimeng_image_multi_v1", tasks[1].Name)
	require.Equal(t, "${"+taskref.Reference("model_image", "render")+".output.prompt}", tasks[1].InputParameters["prompt"])
	require.Equal(t, "${workflow.input.job_id}", tasks[1].InputParameters["job_id"])
	require.Equal(t, "model_image", tasks[1].InputParameters["step_key"])
	require.Equal(t, ir.Steps[0].Capability.Key, tasks[1].InputParameters["capability_key"])
	require.Equal(t, ir.Steps[0].Capability.Version, tasks[1].InputParameters["capability_version"])
	require.Equal(t, []any{
		"${workflow.input.inputs.full_body.asset_ref}",
		"${workflow.input.inputs.clothing.asset_ref}",
	}, tasks[1].InputParameters["images"])
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
	require.Equal(t, "DO_WHILE", definition.Tasks[2].Type)
	require.Equal(t, RenderTemplateTaskName, definition.Tasks[2].LoopOver[0].Name)
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
	require.Len(t, definition.Tasks, 2)
	require.Equal(t, "HUMAN", definition.Tasks[0].LoopOver[2].Type)
	require.Equal(t, approvalRef("image"), definition.Tasks[0].LoopOver[2].TaskReferenceName)
	require.Equal(t, "${"+approvalRef("image")+".output.selected_candidate}", definition.Tasks[1].InputParameters["selected_candidate"])
}

func TestCompileSingleExpectedCandidateStillGuardsActualMultipleCandidates(t *testing.T) {
	step := minimalIRStep("image", 1, nil)
	step.CandidateCount = 1
	step.ApprovalPolicy = "when_multiple_candidates"
	definition, err := CompileExecutableIR(executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: "runtime-review", WorkflowVersion: 1, OutputStepKey: "image", Steps: []executable.ExecutableStepIR{step}})
	require.NoError(t, err)
	gate := definition.Tasks[0].LoopOver[2]
	require.Equal(t, "SWITCH", gate.Type)
	require.Equal(t, "javascript", gate.EvaluatorType)
	require.Equal(t, "${"+taskref.Reference("image", "invoke")+".output.candidates}", gate.InputParameters["candidates"])
	require.Contains(t, gate.Expression, "$.candidates.length > 1")
	require.Empty(t, gate.DecisionCases["automatic"])
	require.Len(t, gate.DecisionCases["review"], 1)
	require.Equal(t, "HUMAN", gate.DecisionCases["review"][0].Type)
	require.Equal(t, approvalRef("image"), gate.DecisionCases["review"][0].TaskReferenceName)
	require.Equal(t, "${"+approvalRef("image")+".output.selected_candidate}", definition.Tasks[1].InputParameters["selected_candidate"])
	require.Contains(t, definition.Tasks[0].LoopCondition, "iteration]")
	require.Contains(t, definition.Tasks[0].LoopCondition, "decision === 'regenerate'")
	for _, task := range definition.Tasks[0].LoopOver {
		require.NotContains(t, task.TaskReferenceName, "__")
	}
}

func TestCompileResumeRestoresSuccessfulPrefixWithoutCapabilitySideEffects(t *testing.T) {
	video := minimalIRStep("video", 4, []string{"first_frame"})
	video.Context = map[string]executable.ValueRef{"frame": {Source: executable.ValueStepOutput, Ref: "first_frame"}}
	video.Invocation["images"] = template.InvocationExpression{Op: template.InvocationAssetRef, Slot: "frame"}
	ir := executable.ExecutableWorkflowIR{
		SchemaVersion: executable.SchemaVersion, WorkflowID: "legacy-video", WorkflowVersion: 1, OutputStepKey: "video",
		Steps: []executable.ExecutableStepIR{
			minimalIRStep("model_image", 1, nil),
			minimalIRStep("style_image", 2, []string{"model_image"}),
			minimalIRStep("first_frame", 3, []string{"style_image"}),
			video,
		},
	}
	definition, err := CompileResumeExecutableIR(ir, map[string]bool{"model_image": true, "style_image": true, "first_frame": true})
	require.NoError(t, err)
	require.Len(t, definition.Tasks, 5)
	for i, key := range []string{"model_image", "style_image", "first_frame"} {
		require.Equal(t, RestoreStepTaskName, definition.Tasks[i].Name)
		require.Equal(t, finalizeRef(key), definition.Tasks[i].TaskReferenceName)
		require.Equal(t, "${workflow.input.job_id}", definition.Tasks[i].InputParameters["job_id"])
	}
	require.Equal(t, RenderTemplateTaskName, definition.Tasks[3].LoopOver[0].Name)
	require.Equal(t, "${step__first_frame__finalize.output.output_resource.asset_ref}", definition.Tasks[3].LoopOver[1].InputParameters["images"])
	require.Equal(t, "${step__video__finalize.output.output_resource_id}", definition.OutputParameters["output_resource_id"])
	_, err = CompileResumeExecutableIR(ir, map[string]bool{"style_image": true})
	require.ErrorContains(t, err, "uncompleted predecessor")
}

func minimalIRStep(key string, order int, dependencies []string) executable.ExecutableStepIR {
	return executable.ExecutableStepIR{
		Key: key, Order: order, DependsOn: dependencies,
		Template:        executable.TemplateIR{ID: "tpl_" + key, Version: 1, Body: "prompt"},
		Context:         map[string]executable.ValueRef{},
		Capability:      executable.CapabilityIR{Key: "cap", Version: 1, ConductorTaskDef: "cap_v1", ExecutionTarget: "personal_worker"},
		Invocation:      template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}},
		OutputAssetKind: "image",
	}
}
