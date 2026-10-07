package publication

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	capabilitydomain "media_agent/media_workflow/biz/domain/capability"
	templatedomain "media_agent/media_workflow/biz/domain/template"
	workflowdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared"
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
	AssetKinds   shared.AssetKindLookup
	Intents      workflowdomain.PublicationRepo
	Workflows    workflowdomain.Repo
	Tx           interface {
		RunInTx(context.Context, func(context.Context) error) error
	}
}

type preparedPublication struct {
	compilation *workflowdomain.Compilation
	definition  compiler.Definition
	tasks       []conductor.TaskDefinition
}

// AcceptPublication does not contact Conductor. Authenticated source ownership
// is checked again before replay; the repository atomically rechecks new intent
// acceptance under the Workflow lock. Recovery consumes the stored command.
func (f *Flow) AcceptPublication(ctx context.Context, workflow *workflowdomain.Workflow, callerID string) (*workflowdomain.Publication, error) {
	if f.Intents == nil || f.Workflows == nil || f.Tx == nil || workflow == nil || callerID == "" {
		return nil, errors.New("publication acceptance unavailable")
	}
	current, err := f.Workflows.Get(ctx, workflow.ID)
	if err != nil || current == nil {
		return nil, errors.New("publication source unavailable")
	}
	if current.CreatedBy != callerID || workflow.CreatedBy != callerID {
		return nil, workflowdomain.ErrPublicationConflict
	}
	previous, err := f.Intents.GetPublication(ctx, workflow.ID, workflow.Version)
	if err != nil {
		return nil, err
	}
	if previous != nil {
		if previous.RequestedBy != callerID || previous.Validate() != nil {
			return nil, workflowdomain.ErrPublicationConflict
		}
		return previous, nil
	}
	if current.Version != workflow.Version || current.Status != workflowdomain.StatusDraft {
		return nil, workflowdomain.ErrPublicationConflict
	}
	// Compile the authorized authoritative source, not a caller's same-version
	// struct that could carry a different DAG or template references.
	prepared, err := f.compile(ctx, current)
	if err != nil {
		return nil, err
	}
	tasks, err := json.Marshal(prepared.tasks)
	if err != nil {
		return nil, errors.New("publication contract encoding failed")
	}
	p := workflowdomain.Publication{ID: fmt.Sprintf("pub_%s_%d", workflow.ID, workflow.Version), WorkflowID: workflow.ID, WorkflowVersion: workflow.Version,
		CompilationID: prepared.compilation.ID, RequestedBy: callerID, DefinitionHash: prepared.compilation.DefinitionHash,
		DefinitionJSON: prepared.compilation.DefinitionJSON, TaskDefinitionsJSON: tasks}
	p.PayloadDigest, err = p.Digest()
	if err != nil {
		return nil, err
	}
	var accepted *workflowdomain.Publication
	err = f.Tx.RunInTx(ctx, func(txCtx context.Context) error {
		var err error
		accepted, _, err = f.Intents.EnqueuePublication(txCtx, prepared.compilation, p)
		return err
	})
	return accepted, err
}

func (f *Flow) compile(ctx context.Context, workflow *workflowdomain.Workflow) (*preparedPublication, error) {
	if workflow == nil || f.Templates == nil || f.Capabilities == nil {
		return nil, errors.New("publication compiler unavailable")
	}
	snapshots := semantic.Snapshots{
		Templates:    make(map[semantic.TemplateRef]semantic.TemplateSnapshot),
		Capabilities: make(map[semantic.CapabilityRef]semantic.CapabilitySnapshot),
	}
	capabilitiesByTask := make(map[string]*capabilitydomain.Capability)
	for _, step := range workflow.Steps {
		template, err := f.Templates.GetVersion(ctx, step.TemplateID, step.TemplateVersion)
		if err != nil {
			return nil, fmt.Errorf("load template %s@%d: %w", step.TemplateID, step.TemplateVersion, err)
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
			Invocation: invocation, NeedsConfirm: template.NeedsConfirm, CandidateCount: template.CandidateCount,
		}
		capability, err := f.Capabilities.Get(ctx, template.CapabilityKey, template.CapabilityVersion)
		if err != nil {
			return nil, fmt.Errorf("load capability %s@%d: %w", template.CapabilityKey, template.CapabilityVersion, err)
		}
		if capability.OutputMediaType != "" {
			if f.AssetKinds == nil {
				return nil, errors.New("publication asset kind lookup unavailable")
			}
			names := []string{template.OutputAssetKind}
			for _, slot := range template.Inputs {
				if slot.ResourceType == "MATERIAL" || slot.ResourceType == "ASSET" {
					names = append(names, slot.AssetKind)
				}
			}
			kinds, missing, err := f.AssetKinds.BatchGet(ctx, names)
			if err != nil || len(missing) != 0 {
				return nil, errors.New("publication asset kind metadata unavailable")
			}
			media := func(name, resourceType string) (capabilitydomain.MediaType, error) {
				kind := kinds[name]
				if kind == nil || kind.ResourceType != resourceType || (kind.Status != "SYSTEM" && kind.Status != "ENABLED") || !capabilitydomain.MediaType(kind.MediaType).Valid() {
					return "", errors.New("publication asset kind metadata incompatible")
				}
				return capabilitydomain.MediaType(kind.MediaType), nil
			}
			outMedia, err := media(template.OutputAssetKind, "ASSET")
			if err != nil {
				return nil, err
			}
			for i, slot := range template.Inputs {
				if slot.ResourceType == "TEXT" {
					continue
				}
				// These aggregate slots expose the selected product image or
				// Virtual IP full-body image, not an AssetKind registry entry.
				if slot.ResourceType == "PRODUCT" || slot.ResourceType == "VIRTUAL_IP" {
					templateInputs[i].MediaType = capabilitydomain.MediaImage
					continue
				}
				templateInputs[i].MediaType, err = media(slot.AssetKind, slot.ResourceType)
				if err != nil {
					return nil, err
				}
			}
			ref := semantic.TemplateRef{ID: step.TemplateID, Version: step.TemplateVersion}
			resolved := snapshots.Templates[ref]
			resolved.OutputMediaType = outMedia
			resolved.Inputs = templateInputs
			snapshots.Templates[ref] = resolved
		}
		inputs := make([]semantic.CapabilityInput, 0, len(capability.Inputs))
		for _, input := range capability.Inputs {
			inputs = append(inputs, semantic.CapabilityInput{Name: input.Name, Type: semantic.ValueType(input.Type), Required: input.Required, MediaType: input.MediaType, MinItems: input.MinItems, MaxItems: input.MaxItems})
		}
		snapshots.Capabilities[semantic.CapabilityRef{Key: capability.Key, Version: capability.Version}] = semantic.CapabilitySnapshot{
			Key: capability.Key, Version: capability.Version, ConductorTaskDef: capability.ConductorTaskDef,
			ExecutionTarget: capability.ExecutionTarget, ManagedPoolKey: capability.ManagedPoolKey,
			Inputs: inputs, OutputAssetKind: capability.OutputAssetKind,
			Provider: capability.Provider, GenerationMethod: capability.GenerationMethod, GenerationType: capability.GenerationType, OutputMediaType: capability.OutputMediaType,
		}
		capabilitiesByTask[capability.ConductorTaskDef] = capability
	}

	ir, err := semantic.Compile(*workflow, snapshots)
	if err != nil {
		return nil, fmt.Errorf("semantic compile: %w", err)
	}
	definition, err := compiler.CompileExecutableIR(ir)
	if err != nil {
		return nil, fmt.Errorf("conductor backend compile: %w", err)
	}
	irJSON, err := executable.CanonicalJSON(ir)
	if err != nil {
		return nil, err
	}
	definitionJSON, err := compiler.CanonicalJSON(definition)
	if err != nil {
		return nil, err
	}
	hash, err := compiler.DefinitionHash(definition)
	if err != nil {
		return nil, err
	}
	compilation := &workflowdomain.Compilation{
		ID: fmt.Sprintf("comp_%s_%d", workflow.ID, workflow.Version), WorkflowID: workflow.ID,
		WorkflowVersion: workflow.Version, ConductorName: definition.Name, ConductorVersion: definition.Version,
		IRJSON: irJSON, DefinitionJSON: definitionJSON, DefinitionHash: hash,
		Status: "registering", CreatedAt: time.Now(),
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
	sort.Slice(taskDefinitions, func(i, j int) bool { return taskDefinitions[i].Name < taskDefinitions[j].Name })
	return &preparedPublication{compilation: compilation, definition: definition, tasks: taskDefinitions}, nil
}

func toTemplateExpression(in workflowdomain.InvocationExpressionSnapshot) templatedomain.InvocationExpression {
	out := templatedomain.InvocationExpression{Op: templatedomain.InvocationOp(in.Op), Slot: in.Slot}
	for _, item := range in.Items {
		out.Items = append(out.Items, toTemplateExpression(item))
	}
	return out
}
