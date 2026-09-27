package compiler

import (
	"fmt"
	"sort"

	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/workflow/executable"
)

const (
	RenderTemplateTaskName = "media_render_template_v1"
	FinalizeStepTaskName   = "media_finalize_step_v1"
)

// CompileExecutableIR lowers the engine-neutral IR into a deterministic
// Conductor definition. Business references have already been validated by the
// semantic compiler; this backend only translates them to Conductor paths.
func CompileExecutableIR(ir executable.ExecutableWorkflowIR) (Definition, error) {
	if ir.SchemaVersion != executable.SchemaVersion {
		return Definition{}, fmt.Errorf("unsupported executable workflow IR schema version %d", ir.SchemaVersion)
	}
	if ir.WorkflowID == "" || ir.WorkflowVersion <= 0 || len(ir.Steps) == 0 {
		return Definition{}, fmt.Errorf("workflow id, positive version, and steps are required")
	}
	byKey := make(map[string]executable.ExecutableStepIR, len(ir.Steps))
	for _, step := range ir.Steps {
		if step.Key == "" || step.Capability.ConductorTaskDef == "" {
			return Definition{}, fmt.Errorf("step key and capability task definition are required")
		}
		if _, exists := byKey[step.Key]; exists {
			return Definition{}, fmt.Errorf("duplicate step key %q", step.Key)
		}
		byKey[step.Key] = step
	}
	if _, ok := byKey[ir.OutputStepKey]; !ok {
		return Definition{}, fmt.Errorf("output step %q not found", ir.OutputStepKey)
	}
	waves, err := executableWaves(byKey)
	if err != nil {
		return Definition{}, err
	}

	definition := Definition{
		Name: "media_workflow_" + sanitize(ir.WorkflowID), Version: ir.WorkflowVersion,
		InputParameters: []string{"inputs", "job_id"}, OutputParameters: map[string]any{}, SchemaVersion: 2,
	}
	for waveIndex, steps := range waves {
		if len(steps) == 1 {
			branch, err := lowerExecutableStep(steps[0])
			if err != nil {
				return Definition{}, err
			}
			definition.Tasks = append(definition.Tasks, branch...)
			continue
		}
		branches := make([][]Task, 0, len(steps))
		joinOn := make([]string, 0, len(steps))
		for _, step := range steps {
			branch, err := lowerExecutableStep(step)
			if err != nil {
				return Definition{}, err
			}
			branches = append(branches, branch)
			joinOn = append(joinOn, finalizeRef(step.Key))
		}
		definition.Tasks = append(definition.Tasks,
			Task{Name: fmt.Sprintf("wave_%d_fork", waveIndex), TaskReferenceName: fmt.Sprintf("wave_%d_fork", waveIndex), Type: "FORK_JOIN", ForkTasks: branches},
			Task{Name: fmt.Sprintf("wave_%d_join", waveIndex), TaskReferenceName: fmt.Sprintf("wave_%d_join", waveIndex), Type: "JOIN", JoinOn: joinOn},
		)
	}
	definition.OutputParameters["output_resource_id"] = fmt.Sprintf("${%s.output.output_resource_id}", finalizeRef(ir.OutputStepKey))
	return definition, nil
}

func lowerExecutableStep(step executable.ExecutableStepIR) ([]Task, error) {
	context := make(map[string]any, len(step.Context))
	for slot, ref := range step.Context {
		context[slot] = lowerValueRef(ref)
	}
	renderRef := "step__" + sanitize(step.Key) + "__render"
	invokeRef := "step__" + sanitize(step.Key) + "__invoke"
	capabilityInput := make(map[string]any, len(step.Invocation))
	for name, expression := range step.Invocation {
		value, err := lowerInvocation(expression, step.Context, renderRef)
		if err != nil {
			return nil, fmt.Errorf("step %q input %q: %w", step.Key, name, err)
		}
		capabilityInput[name] = value
	}
	capabilityInput["job_id"] = "${workflow.input.job_id}"
	capabilityInput["step_key"] = step.Key
	tasks := []Task{
		{
			Name: RenderTemplateTaskName, TaskReferenceName: renderRef, Type: "SIMPLE",
			InputParameters: map[string]any{
				"template": map[string]any{"id": step.Template.ID, "version": step.Template.Version, "body": step.Template.Body},
				"context":  context,
			},
		},
		{Name: step.Capability.ConductorTaskDef, TaskReferenceName: invokeRef, Type: "SIMPLE", InputParameters: capabilityInput},
	}
	finalizeInput := map[string]any{
		"step_key": step.Key, "candidates": fmt.Sprintf("${%s.output.candidates}", invokeRef),
		"output_asset_kind": step.OutputAssetKind, "approval_policy": step.ApprovalPolicy, "reuse_policy": step.ReusePolicy,
		"job_id": "${workflow.input.job_id}",
	}
	if step.ApprovalRequired {
		tasks = append(tasks, Task{
			Name: "human_approval_" + sanitize(step.Key), TaskReferenceName: approvalRef(step.Key), Type: "HUMAN",
			InputParameters: map[string]any{
				"step_key": step.Key, "candidates": fmt.Sprintf("${%s.output.candidates}", invokeRef),
				"approval_policy": step.ApprovalPolicy, "job_id": "${workflow.input.job_id}",
			},
		})
		finalizeInput["selected_candidate"] = fmt.Sprintf("${%s.output.selected_candidate}", approvalRef(step.Key))
	}
	tasks = append(tasks, Task{Name: FinalizeStepTaskName, TaskReferenceName: finalizeRef(step.Key), Type: "SIMPLE", InputParameters: finalizeInput})
	return tasks, nil
}

func lowerInvocation(expression template.InvocationExpression, context map[string]executable.ValueRef, renderRef string) (any, error) {
	switch expression.Op {
	case template.InvocationRenderTemplate:
		return fmt.Sprintf("${%s.output.prompt}", renderRef), nil
	case template.InvocationAssetRef:
		ref, ok := context[expression.Slot]
		if !ok {
			return nil, fmt.Errorf("slot %q is absent from context", expression.Slot)
		}
		return lowerAssetRef(ref), nil
	case template.InvocationList:
		values := make([]any, 0, len(expression.Items))
		for _, item := range expression.Items {
			value, err := lowerInvocation(item, context, renderRef)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported expression op %q", expression.Op)
	}
}

func lowerValueRef(ref executable.ValueRef) any {
	switch ref.Source {
	case executable.ValueWorkflowInput:
		return fmt.Sprintf("${workflow.input.inputs.%s}", ref.Ref)
	case executable.ValueStepOutput:
		return fmt.Sprintf("${%s.output.output_resource}", finalizeRef(ref.Ref))
	default:
		return ref.ConstValue
	}
}

func lowerAssetRef(ref executable.ValueRef) any {
	switch ref.Source {
	case executable.ValueWorkflowInput:
		return fmt.Sprintf("${workflow.input.inputs.%s.asset_ref}", ref.Ref)
	case executable.ValueStepOutput:
		return fmt.Sprintf("${%s.output.output_resource.asset_ref}", finalizeRef(ref.Ref))
	default:
		return ref.ConstValue
	}
}

func finalizeRef(stepKey string) string { return "step__" + sanitize(stepKey) + "__finalize" }
func approvalRef(stepKey string) string { return "step__" + sanitize(stepKey) + "__approval" }

func executableWaves(byKey map[string]executable.ExecutableStepIR) ([][]executable.ExecutableStepIR, error) {
	indegree := make(map[string]int, len(byKey))
	children := make(map[string][]string, len(byKey))
	for key := range byKey {
		indegree[key] = 0
	}
	for key, step := range byKey {
		seen := map[string]struct{}{}
		for _, dependency := range step.DependsOn {
			if _, ok := byKey[dependency]; !ok {
				return nil, fmt.Errorf("step %q references unknown dependency %q", key, dependency)
			}
			if _, duplicate := seen[dependency]; duplicate {
				continue
			}
			seen[dependency] = struct{}{}
			indegree[key]++
			children[dependency] = append(children[dependency], key)
		}
	}
	remaining := len(byKey)
	var waves [][]executable.ExecutableStepIR
	for remaining > 0 {
		var ready []executable.ExecutableStepIR
		for key, degree := range indegree {
			if degree == 0 {
				ready = append(ready, byKey[key])
			}
		}
		if len(ready) == 0 {
			return nil, fmt.Errorf("executable workflow IR contains a cycle")
		}
		sort.Slice(ready, func(i, j int) bool {
			if ready[i].Order != ready[j].Order {
				return ready[i].Order < ready[j].Order
			}
			return ready[i].Key < ready[j].Key
		})
		waves = append(waves, ready)
		for _, step := range ready {
			delete(indegree, step.Key)
			remaining--
			for _, child := range children[step.Key] {
				indegree[child]--
			}
		}
	}
	return waves, nil
}
