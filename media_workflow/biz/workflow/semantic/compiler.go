// Package semantic compiles business workflow snapshots into an engine-neutral
// Executable Workflow IR.
package semantic

import (
	"fmt"
	"sort"
	"strings"

	templatedomain "media_agent/media_workflow/biz/domain/template"
	workflowdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

type ValueType string

const (
	TypeString       ValueType = "string"
	TypeResourceRef  ValueType = "resource_ref"
	TypeResourceList ValueType = "resource_ref_list"
)

type CapabilityInput struct {
	Name     string
	Type     ValueType
	Required bool
}

type CapabilitySnapshot struct {
	Key              string
	Version          int
	ConductorTaskDef string
	Inputs           []CapabilityInput
	OutputAssetKind  string
}

type TemplateSlot struct {
	Name     string
	Resource bool
	Required bool
}

type TemplateSnapshot struct {
	ID                string
	Version           int
	Body              string
	Inputs            []TemplateSlot
	OutputAssetKind   string
	CapabilityKey     string
	CapabilityVersion int
	Invocation        templatedomain.InvocationMapping
	NeedsConfirm      bool
	CandidateCount    int
}

type TemplateRef struct {
	ID      string
	Version int
}

type CapabilityRef struct {
	Key     string
	Version int
}

type Snapshots struct {
	Templates    map[TemplateRef]TemplateSnapshot
	Capabilities map[CapabilityRef]CapabilitySnapshot
}

func Compile(wf workflowdomain.Workflow, snapshots Snapshots) (executable.ExecutableWorkflowIR, error) {
	if strings.TrimSpace(wf.ID) == "" || wf.Version <= 0 {
		return executable.ExecutableWorkflowIR{}, fmt.Errorf("workflow id and positive version are required")
	}
	if len(wf.Steps) == 0 {
		return executable.ExecutableWorkflowIR{}, fmt.Errorf("workflow must contain at least one step")
	}

	steps := append([]workflowdomain.Step(nil), wf.Steps...)
	sort.Slice(steps, func(i, j int) bool {
		if steps[i].Order != steps[j].Order {
			return steps[i].Order < steps[j].Order
		}
		return steps[i].Key < steps[j].Key
	})

	ir := executable.ExecutableWorkflowIR{
		SchemaVersion: executable.SchemaVersion,
		WorkflowID:    wf.ID, WorkflowVersion: wf.Version, OutputStepKey: wf.OutputStepKey,
		Steps: make([]executable.ExecutableStepIR, 0, len(steps)),
	}
	seen := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		if _, duplicate := seen[step.Key]; duplicate {
			return executable.ExecutableWorkflowIR{}, fmt.Errorf("duplicate step key %q", step.Key)
		}
		seen[step.Key] = struct{}{}

		tpl, ok := snapshots.Templates[TemplateRef{ID: step.TemplateID, Version: step.TemplateVersion}]
		if !ok {
			return executable.ExecutableWorkflowIR{}, fmt.Errorf("step %q template %s@%d not found", step.Key, step.TemplateID, step.TemplateVersion)
		}
		capability, ok := snapshots.Capabilities[CapabilityRef{Key: tpl.CapabilityKey, Version: tpl.CapabilityVersion}]
		if !ok {
			return executable.ExecutableWorkflowIR{}, fmt.Errorf("step %q capability %s@%d not found", step.Key, tpl.CapabilityKey, tpl.CapabilityVersion)
		}
		if tpl.OutputAssetKind != capability.OutputAssetKind {
			return executable.ExecutableWorkflowIR{}, fmt.Errorf("step %q output asset kind %q does not match capability %q", step.Key, tpl.OutputAssetKind, capability.OutputAssetKind)
		}

		slots := make(map[string]TemplateSlot, len(tpl.Inputs))
		for _, slot := range tpl.Inputs {
			slots[slot.Name] = slot
		}
		context, dependencies, err := compileBindings(step, slots, seen)
		if err != nil {
			return executable.ExecutableWorkflowIR{}, err
		}
		if err := validateInvocation(tpl.Invocation, slots, capability.Inputs); err != nil {
			return executable.ExecutableWorkflowIR{}, fmt.Errorf("step %q invocation mapping: %w", step.Key, err)
		}

		ir.Steps = append(ir.Steps, executable.ExecutableStepIR{
			Key: step.Key, Order: step.Order, DependsOn: dependencies,
			Template:   executable.TemplateIR{ID: tpl.ID, Version: tpl.Version, Body: tpl.Body},
			Context:    context,
			Capability: executable.CapabilityIR{Key: capability.Key, Version: capability.Version, ConductorTaskDef: capability.ConductorTaskDef},
			Invocation: cloneMapping(tpl.Invocation), OutputAssetKind: tpl.OutputAssetKind,
			ApprovalPolicy:   string(step.ApprovalPolicy),
			ApprovalRequired: tpl.NeedsConfirm || tpl.CandidateCount > 1 || step.ApprovalPolicy == workflowdomain.ApprovalAlways,
			CandidateCount:   tpl.CandidateCount, ReusePolicy: string(step.ReusePolicy),
		})
	}
	if _, ok := seen[wf.OutputStepKey]; !ok {
		return executable.ExecutableWorkflowIR{}, fmt.Errorf("output step %q not found", wf.OutputStepKey)
	}
	return ir, nil
}

func compileBindings(step workflowdomain.Step, slots map[string]TemplateSlot, seen map[string]struct{}) (map[string]executable.ValueRef, []string, error) {
	context := make(map[string]executable.ValueRef, len(step.SlotBindings))
	dependencySet := map[string]struct{}{}
	for _, binding := range step.SlotBindings {
		if _, ok := slots[binding.ToInput]; !ok {
			return nil, nil, fmt.Errorf("step %q binding targets unknown slot %q", step.Key, binding.ToInput)
		}
		if _, duplicate := context[binding.ToInput]; duplicate {
			return nil, nil, fmt.Errorf("step %q has duplicate binding for slot %q", step.Key, binding.ToInput)
		}
		switch binding.Source {
		case workflowdomain.SourceWorkflowInput:
			if strings.TrimSpace(binding.Ref) == "" {
				return nil, nil, fmt.Errorf("step %q workflow input binding %q requires explicit ref", step.Key, binding.ToInput)
			}
			context[binding.ToInput] = executable.ValueRef{Source: executable.ValueWorkflowInput, Ref: binding.Ref}
		case workflowdomain.SourceStepOutput:
			if _, ok := seen[binding.Ref]; !ok || binding.Ref == step.Key {
				return nil, nil, fmt.Errorf("step %q references unavailable step %q", step.Key, binding.Ref)
			}
			context[binding.ToInput] = executable.ValueRef{Source: executable.ValueStepOutput, Ref: binding.Ref}
			dependencySet[binding.Ref] = struct{}{}
		case workflowdomain.SourceConst:
			context[binding.ToInput] = executable.ValueRef{Source: executable.ValueConst, ConstValue: binding.ConstValue}
		default:
			return nil, nil, fmt.Errorf("step %q binding %q has unknown source %q", step.Key, binding.ToInput, binding.Source)
		}
	}
	for name, slot := range slots {
		if slot.Required {
			if _, ok := context[name]; !ok {
				return nil, nil, fmt.Errorf("step %q required slot %q is not bound", step.Key, name)
			}
		}
	}
	dependencies := make([]string, 0, len(dependencySet))
	for dependency := range dependencySet {
		dependencies = append(dependencies, dependency)
	}
	sort.Strings(dependencies)
	return context, dependencies, nil
}

func validateInvocation(mapping templatedomain.InvocationMapping, slots map[string]TemplateSlot, inputs []CapabilityInput) error {
	declared := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		declared[input.Name] = struct{}{}
		expr, ok := mapping[input.Name]
		if !ok {
			if input.Required {
				return fmt.Errorf("required capability input %q is not mapped", input.Name)
			}
			continue
		}
		actual, err := inferExpression(expr, slots)
		if err != nil {
			return fmt.Errorf("input %q: %w", input.Name, err)
		}
		if actual != input.Type {
			return fmt.Errorf("input %q expects %s, got %s", input.Name, input.Type, actual)
		}
	}
	for name := range mapping {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("capability input %q is not declared", name)
		}
	}
	return nil
}

func inferExpression(expr templatedomain.InvocationExpression, slots map[string]TemplateSlot) (ValueType, error) {
	switch expr.Op {
	case templatedomain.InvocationRenderTemplate:
		if expr.Slot != "" || len(expr.Items) != 0 {
			return "", fmt.Errorf("render_template does not accept slot or items")
		}
		return TypeString, nil
	case templatedomain.InvocationAssetRef:
		if expr.Slot == "" || len(expr.Items) != 0 {
			return "", fmt.Errorf("asset_ref requires one slot and no items")
		}
		slot, ok := slots[expr.Slot]
		if !ok {
			return "", fmt.Errorf("asset_ref references unknown slot %q", expr.Slot)
		}
		if !slot.Resource {
			return "", fmt.Errorf("asset_ref slot %q is not a resource", expr.Slot)
		}
		return TypeResourceRef, nil
	case templatedomain.InvocationList:
		if expr.Slot != "" || len(expr.Items) == 0 {
			return "", fmt.Errorf("list must contain at least one item")
		}
		for _, item := range expr.Items {
			itemType, err := inferExpression(item, slots)
			if err != nil {
				return "", err
			}
			if itemType != TypeResourceRef {
				return "", fmt.Errorf("list item expects %s, got %s", TypeResourceRef, itemType)
			}
		}
		return TypeResourceList, nil
	default:
		return "", fmt.Errorf("unsupported expression op %q", expr.Op)
	}
}

func cloneMapping(source templatedomain.InvocationMapping) templatedomain.InvocationMapping {
	cloned := make(templatedomain.InvocationMapping, len(source))
	for key, expression := range source {
		cloned[key] = cloneExpression(expression)
	}
	return cloned
}

func cloneExpression(source templatedomain.InvocationExpression) templatedomain.InvocationExpression {
	cloned := source
	cloned.Items = make([]templatedomain.InvocationExpression, len(source.Items))
	for i, item := range source.Items {
		cloned.Items[i] = cloneExpression(item)
	}
	return cloned
}
