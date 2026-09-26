package publication

import (
	"context"
	"fmt"
	"time"

	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	templatedomain "media_agent/media_workflow/biz/domain/template"
	workflowdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/conductor"
	"media_agent/media_workflow/biz/workflow/executable"
	"media_agent/media_workflow/biz/workflow/finalize_task"
	"media_agent/media_workflow/biz/workflow/render_task"
	"media_agent/media_workflow/biz/workflow/semantic"
)

type Flow struct {
	Templates    workflowdomain.TemplateLookup
	Capabilities capabilitydomain.Repo
	Compilations workflowdomain.CompilationRepo
	Conductor    *conductor.Client
}

func (f *Flow) CompileAndRegister(ctx context.Context, workflow *workflowdomain.Workflow) error {
	if f.Conductor == nil || !f.Conductor.Enabled() {
		return fmt.Errorf("CONDUCTOR_URL is required to publish executable workflows")
	}
	snapshots := semantic.Snapshots{
		Templates:    make(map[semantic.TemplateRef]semantic.TemplateSnapshot),
		Capabilities: make(map[semantic.CapabilityRef]semantic.CapabilitySnapshot),
	}
	capabilitiesByTask := make(map[string]*capabilitydomain.Capability)
	for _, step := range workflow.Steps {
		template, err := f.Templates.GetVersion(ctx, step.TemplateID, step.TemplateVersion)
		if err != nil {
			return fmt.Errorf("load template %s@%d: %w", step.TemplateID, step.TemplateVersion, err)
		}
		invocation := make(templatedomain.InvocationMapping, len(template.InvocationMapping))
		for name, expression := range template.InvocationMapping {
			invocation[name] = toTemplateExpression(expression)
		}
		templateInputs := make([]semantic.TemplateSlot, 0, len(template.Inputs))
		for _, slot := range template.Inputs {
			templateInputs = append(templateInputs, semantic.TemplateSlot{
				Name: slot.Name, Resource: slot.ResourceType != "TEXT", Required: slot.Required,
			})
		}
		snapshots.Templates[semantic.TemplateRef{ID: step.TemplateID, Version: step.TemplateVersion}] = semantic.TemplateSnapshot{
			ID: step.TemplateID, Version: step.TemplateVersion, Body: template.PromptBody,
			Inputs: templateInputs, OutputAssetKind: template.OutputAssetKind,
			CapabilityKey: template.CapabilityKey, CapabilityVersion: template.CapabilityVersion,
			Invocation: invocation,
		}
		capability, err := f.Capabilities.Get(ctx, template.CapabilityKey, template.CapabilityVersion)
		if err != nil {
			return fmt.Errorf("load capability %s@%d: %w", template.CapabilityKey, template.CapabilityVersion, err)
		}
		inputs := make([]semantic.CapabilityInput, 0, len(capability.Inputs))
		for _, input := range capability.Inputs {
			inputs = append(inputs, semantic.CapabilityInput{Name: input.Name, Type: semantic.ValueType(input.Type), Required: input.Required})
		}
		snapshots.Capabilities[semantic.CapabilityRef{Key: capability.Key, Version: capability.Version}] = semantic.CapabilitySnapshot{
			Key: capability.Key, Version: capability.Version, ConductorTaskDef: capability.ConductorTaskDef,
			Inputs: inputs, OutputAssetKind: capability.OutputAssetKind,
		}
		capabilitiesByTask[capability.ConductorTaskDef] = capability
	}

	ir, err := semantic.Compile(*workflow, snapshots)
	if err != nil {
		return fmt.Errorf("semantic compile: %w", err)
	}
	definition, err := compiler.CompileExecutableIR(ir)
	if err != nil {
		return fmt.Errorf("conductor backend compile: %w", err)
	}
	irJSON, err := executable.CanonicalJSON(ir)
	if err != nil {
		return err
	}
	definitionJSON, err := compiler.CanonicalJSON(definition)
	if err != nil {
		return err
	}
	hash, err := compiler.DefinitionHash(definition)
	if err != nil {
		return err
	}
	compilation := &workflowdomain.Compilation{
		ID: fmt.Sprintf("comp_%s_%d", workflow.ID, workflow.Version), WorkflowID: workflow.ID,
		WorkflowVersion: workflow.Version, ConductorName: definition.Name, ConductorVersion: definition.Version,
		IRJSON: irJSON, DefinitionJSON: definitionJSON, DefinitionHash: hash,
		Status: "registering", CreatedAt: time.Now(),
	}
	if err := f.Compilations.Save(ctx, compilation); err != nil {
		return fmt.Errorf("save compilation: %w", err)
	}
	taskDefinitions := []conductor.TaskDefinition{
		{Name: render_task.TaskDefinitionName, RetryCount: 2, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, TimeoutPolicy: "TIME_OUT_WF"},
		{Name: finalize_task.TaskDefinitionName, RetryCount: 2, RetryLogic: "FIXED", RetryDelaySeconds: 1, TimeoutSeconds: 60, ResponseTimeoutSeconds: 30, TimeoutPolicy: "TIME_OUT_WF"},
	}
	for _, capability := range capabilitiesByTask {
		taskDefinitions = append(taskDefinitions, conductor.TaskDefinition{
			Name: capability.ConductorTaskDef, RetryCount: capability.RetryCount,
			RetryLogic: capability.RetryLogic, RetryDelaySeconds: capability.RetryDelaySeconds,
			TimeoutSeconds: capability.TimeoutSeconds, ResponseTimeoutSeconds: capability.TimeoutSeconds,
			TimeoutPolicy: "TIME_OUT_WF",
		})
	}
	if err := f.Conductor.RegisterTaskDefinitions(ctx, taskDefinitions); err != nil {
		_ = f.Compilations.MarkFailed(ctx, compilation.ID, err.Error())
		return err
	}
	if err := f.Conductor.RegisterWorkflow(ctx, definition); err != nil {
		_ = f.Compilations.MarkFailed(ctx, compilation.ID, err.Error())
		return err
	}
	if err := f.Compilations.MarkRegistered(ctx, compilation.ID); err != nil {
		return err
	}
	return nil
}

func toTemplateExpression(in workflowdomain.InvocationExpressionSnapshot) templatedomain.InvocationExpression {
	out := templatedomain.InvocationExpression{Op: templatedomain.InvocationOp(in.Op), Slot: in.Slot}
	for _, item := range in.Items {
		out.Items = append(out.Items, toTemplateExpression(item))
	}
	return out
}
