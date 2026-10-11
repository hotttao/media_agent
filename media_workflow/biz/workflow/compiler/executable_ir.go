package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	template "media_agent/media_workflow/biz/domain/template"
	"media_agent/media_workflow/biz/shared/taskref"
	"media_agent/media_workflow/biz/workflow/executable"
	"media_agent/media_workflow/biz/workflow/reuse_task"
)

const (
	RenderTemplateTaskName = "media_render_template_v1"
	FinalizeStepTaskName   = "media_finalize_step_v1"
	RestoreStepTaskName    = "media_restore_historical_step_v1"
)

// CompileExecutableIR lowers the engine-neutral IR into a deterministic
// Conductor definition. Business references have already been validated by the
// semantic compiler; this backend only translates them to Conductor paths.
func CompileExecutableIR(ir executable.ExecutableWorkflowIR) (Definition, error) {
	return compileExecutableIR(ir, nil)
}

// CompileResumeExecutableIR replaces only a dependency-closed set of frozen
// successful historical steps with trusted restore tasks. Other steps retain
// their original Render/Capability/Approval/Finalize plan.
func CompileResumeExecutableIR(ir executable.ExecutableWorkflowIR, restored map[string]bool) (Definition, error) {
	if len(restored) == 0 || restored[ir.OutputStepKey] {
		return Definition{}, fmt.Errorf("resume requires a non-empty proper completed prefix")
	}
	byKey := make(map[string]executable.ExecutableStepIR, len(ir.Steps))
	keys := make([]string, 0, len(restored))
	for _, step := range ir.Steps {
		byKey[step.Key] = step
	}
	for key := range restored {
		step, exists := byKey[key]
		if !exists {
			return Definition{}, fmt.Errorf("unknown restored step %q", key)
		}
		for _, dependency := range step.DependsOn {
			if !restored[dependency] {
				return Definition{}, fmt.Errorf("restored step %q has uncompleted predecessor %q", key, dependency)
			}
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	definition, err := compileExecutableIR(ir, restored)
	if err != nil {
		return Definition{}, err
	}
	hash := sha256.Sum256([]byte(strings.Join(keys, "\x00")))
	definition.Name = "media_workflow_resume_" + sanitize(ir.WorkflowID) + "_" + hex.EncodeToString(hash[:6])
	return definition, nil
}

func compileExecutableIR(ir executable.ExecutableWorkflowIR, restored map[string]bool) (Definition, error) {
	if ir.SchemaVersion != executable.SchemaVersion && ir.SchemaVersion != executable.LegacySchemaVersion {
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
			branch, err := lowerStepOrRestore(steps[0], restored, ir.SchemaVersion == executable.SchemaVersion && restored == nil)
			if err != nil {
				return Definition{}, err
			}
			definition.Tasks = append(definition.Tasks, branch...)
			continue
		}
		branches := make([][]Task, 0, len(steps))
		joinOn := make([]string, 0, len(steps))
		for _, step := range steps {
			branch, err := lowerStepOrRestore(step, restored, ir.SchemaVersion == executable.SchemaVersion && restored == nil)
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

func lowerStepOrRestore(step executable.ExecutableStepIR, restored map[string]bool, enableReuse bool) ([]Task, error) {
	if restored[step.Key] {
		return []Task{{
			Name: RestoreStepTaskName, TaskReferenceName: finalizeRef(step.Key), Type: "SIMPLE",
			InputParameters: map[string]any{"job_id": "${workflow.input.job_id}", "step_key": step.Key},
		}}, nil
	}
	tasks, err := lowerExecutableStep(step)
	if err != nil || !enableReuse {
		return tasks, err
	}
	return lowerReusableStep(step, tasks)
}

// Only newly compiled v4 native definitions opt in. Frozen v3 definitions and
// historical restore compilation retain the exact original execution plan.
func lowerReusableStep(step executable.ExecutableStepIR, generated []Task) ([]Task, error) {
	if step.ReusePolicy != "reuse_if_available" && step.ReusePolicy != "force_new" {
		return nil, fmt.Errorf("step %q has an invalid reuse policy", step.Key)
	}
	if len(generated) != 2 || generated[0].Type != "DO_WHILE" {
		return nil, fmt.Errorf("step %q has no generation loop", step.Key)
	}
	prepareRef := taskref.Reference(step.Key, "prepare")
	checkRef := taskref.Reference(step.Key, "reuse")
	context := make(map[string]any, len(step.Context))
	for name, ref := range step.Context {
		context[name] = lowerValueRef(ref)
	}
	parameters := make(map[string]any, len(step.Invocation))
	for name, expression := range step.Invocation {
		value, err := lowerInvocation(expression, step.Context, prepareRef)
		if err != nil {
			return nil, err
		}
		parameters[name] = value
	}
	identity := reuse_task.Identity{TemplateID: step.Template.ID, TemplateVersion: step.Template.Version, CapabilityKey: step.Capability.Key, CapabilityVersion: step.Capability.Version, CandidateCount: step.CandidateCount, OutputAssetKind: step.OutputAssetKind, Inputs: step.Capability.Inputs}
	// Definition parameters must be JSON values, not nested Go structs. Struct
	// field order differs from maps after persistence/decode and would invalidate
	// the immutable Definition hash without any semantic change.
	encoded, err := json.Marshal(identity)
	if err != nil {
		return nil, err
	}
	var frozenIdentity map[string]any
	if err := json.Unmarshal(encoded, &frozenIdentity); err != nil {
		return nil, err
	}
	generated[1].InputParameters["fingerprint_identity"] = frozenIdentity
	generated[1].InputParameters["reuse_result"] = fmt.Sprintf("${%s.output}", checkRef)
	return []Task{
		{Name: RenderTemplateTaskName, TaskReferenceName: prepareRef, Type: "SIMPLE", InputParameters: map[string]any{"template": map[string]any{"id": step.Template.ID, "version": step.Template.Version, "body": step.Template.Body}, "context": context}},
		{Name: reuse_task.TaskDefinitionName, TaskReferenceName: checkRef, Type: "SIMPLE", InputParameters: map[string]any{"job_id": "${workflow.input.job_id}", "step_key": step.Key, "policy": step.ReusePolicy, "identity": frozenIdentity, "provider_input": parameters}},
		{Name: "reuse_gate_" + sanitize(step.Key), TaskReferenceName: taskref.Reference(step.Key, "reuse_gate"), Type: "SWITCH", EvaluatorType: "javascript", Expression: "$.hit === true ? 'reuse' : 'generate'", InputParameters: map[string]any{"hit": fmt.Sprintf("${%s.output.hit}", checkRef)}, DecisionCases: map[string][]Task{"reuse": {}, "generate": {generated[0]}}},
		generated[1],
	}, nil
}

func lowerExecutableStep(step executable.ExecutableStepIR) ([]Task, error) {
	context := make(map[string]any, len(step.Context))
	for slot, ref := range step.Context {
		context[slot] = lowerValueRef(ref)
	}
	renderRef := taskref.Reference(step.Key, "render")
	loopRef := taskref.Reference(step.Key, "loop")
	invokeRef := taskref.Reference(step.Key, "invoke")
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
	capabilityInput["capability_key"] = step.Capability.Key
	capabilityInput["capability_version"] = step.Capability.Version
	tasks := []Task{
		{
			Name: RenderTemplateTaskName, TaskReferenceName: renderRef, Type: "SIMPLE",
			InputParameters: map[string]any{
				"template":            map[string]any{"id": step.Template.ID, "version": step.Template.Version, "body": step.Template.Body},
				"context":             context,
				"feedback_mode":       "last_regeneration_v1",
				"generation":          fmt.Sprintf("${%s.output.iteration}", loopRef),
				"previous_generation": fmt.Sprintf("${%s.iteration}", approvalRef(step.Key)),
				"previous_review":     fmt.Sprintf("${%s.output}", approvalRef(step.Key)),
			},
		},
		{Name: step.Capability.ConductorTaskDef, TaskReferenceName: invokeRef, Type: "SIMPLE", InputParameters: capabilityInput},
	}
	finalizeInput := map[string]any{
		"step_key": step.Key, "candidates": fmt.Sprintf("${%s.output.candidates}", invokeRef),
		"output_asset_kind": step.OutputAssetKind, "approval_policy": step.ApprovalPolicy, "reuse_policy": step.ReusePolicy,
		"job_id": "${workflow.input.job_id}",
	}
	approval := Task{
		Name: "human_approval_" + sanitize(step.Key), TaskReferenceName: approvalRef(step.Key), Type: "HUMAN",
		InputParameters: map[string]any{
			"step_key": step.Key, "candidates": fmt.Sprintf("${%s.output.candidates}", invokeRef),
			"approval_policy": step.ApprovalPolicy, "job_id": "${workflow.input.job_id}",
			"feedback_mode": "last_regeneration_v1",
		},
	}
	if step.ApprovalRequired || step.ApprovalPolicy == "always" || step.CandidateCount > 1 {
		tasks = append(tasks, approval)
	} else {
		// Expected candidate_count is not a guarantee about Provider output.
		// Actual multiple candidates must always wait for a human decision.
		tasks = append(tasks, Task{
			Name: "approval_gate_" + sanitize(step.Key), TaskReferenceName: taskref.Reference(step.Key, "gate"), Type: "SWITCH",
			EvaluatorType: "javascript", Expression: "($.candidates && $.candidates.length > 1) ? 'review' : 'automatic'",
			InputParameters: map[string]any{"candidates": fmt.Sprintf("${%s.output.candidates}", invokeRef)},
			DecisionCases:   map[string][]Task{"review": {approval}, "automatic": {}},
		})
	}
	finalizeInput["selected_candidate"] = fmt.Sprintf("${%s.output.selected_candidate}", approvalRef(step.Key))
	// Read the current iteration snapshot, not the latest HUMAN across all
	// iterations: a subsequent automatic round must not reuse an earlier
	// regenerate decision and loop forever.
	condition := fmt.Sprintf("(function(){ var round = $.%s[$.%s.iteration]; return !!(round && round.%s && round.%s.decision === 'regenerate'); })();", loopRef, loopRef, approvalRef(step.Key), approvalRef(step.Key))
	return []Task{
		{Name: "generation_loop_" + sanitize(step.Key), TaskReferenceName: loopRef, Type: "DO_WHILE", LoopOver: tasks, LoopCondition: condition},
		{Name: FinalizeStepTaskName, TaskReferenceName: finalizeRef(step.Key), Type: "SIMPLE", InputParameters: finalizeInput},
	}, nil
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
func approvalRef(stepKey string) string { return taskref.Reference(stepKey, "approval") }

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
