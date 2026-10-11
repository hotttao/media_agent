package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"gorm.io/gorm"
	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/workflow"
)

// Read-only decoding of frozen evidence, not a compiler or engine adapter.
// IR omitted display names and entry schema: never substitute today's head.
type storedWorkflowIR struct {
	SchemaVersion   int    `json:"schema_version"`
	WorkflowID      string `json:"workflow_id"`
	WorkflowVersion int    `json:"workflow_version"`
	OutputStepKey   string `json:"output_step_key"`
	Steps           []struct {
		Key      string `json:"key"`
		Order    int    `json:"order"`
		Template struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"template"`
		Context map[string]struct {
			Source     string `json:"source"`
			Ref        string `json:"ref"`
			ConstValue string `json:"const_value"`
		} `json:"context"`
		ApprovalPolicy string `json:"approval_policy"`
		ReusePolicy    string `json:"reuse_policy"`
	} `json:"steps"`
}

type storedTemplateSlot struct {
	Name         string
	ResourceType string
	Required     bool
	AssetKind    string
}

func (r *Repo) compiledDefinitionVersion(ctx context.Context, id string, version int) (*domain.DefinitionVersion, error) {
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	var compiled model.WorkflowCompilation
	err := db.Where("workflow_id=? AND workflow_version=? AND status='registered'", id, version).First(&compiled).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrVersionNotFound
	}
	if err != nil {
		return nil, err
	}
	var ir storedWorkflowIR
	if json.Unmarshal(compiled.IRJSON, &ir) != nil || (ir.SchemaVersion != 3 && ir.SchemaVersion != 4) || ir.WorkflowID != id || ir.WorkflowVersion != version || len(ir.Steps) == 0 {
		return nil, domain.ErrVersionNotFound
	}
	w := &domain.Workflow{ID: id, Version: version, OutputStepKey: ir.OutputStepKey, UpdatedAt: compiled.CreatedAt}
	entries := map[string]domain.InputSchemaEntry{}
	keys := map[string]bool{}
	for _, step := range ir.Steps {
		if step.Key == "" || keys[step.Key] || step.Template.ID == "" || step.Template.Version <= 0 {
			return nil, domain.ErrVersionNotFound
		}
		keys[step.Key] = true
		if step.ApprovalPolicy != string(domain.ApprovalWhenMultipleCandidates) && step.ApprovalPolicy != string(domain.ApprovalAlways) {
			return nil, domain.ErrVersionNotFound
		}
		if step.ReusePolicy != string(domain.ReuseIfAvailable) && step.ReusePolicy != string(domain.ForceNew) {
			return nil, domain.ErrVersionNotFound
		}
		st := domain.Step{Key: step.Key, Order: step.Order, TemplateID: step.Template.ID, TemplateVersion: step.Template.Version, ApprovalPolicy: domain.ApprovalPolicy(step.ApprovalPolicy), ReusePolicy: domain.ReusePolicy(step.ReusePolicy)}
		var template model.TemplateVersion
		err := db.Where("template_id=? AND version=?", step.Template.ID, step.Template.Version).First(&template).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrVersionNotFound
		}
		if err != nil {
			return nil, err
		}
		var slots []storedTemplateSlot
		if json.Unmarshal(template.InputsSnapshot, &slots) != nil {
			return nil, domain.ErrVersionNotFound
		}
		slotMap := map[string]storedTemplateSlot{}
		for _, slot := range slots {
			slotMap[slot.Name] = slot
		}
		var names []string
		for name := range step.Context {
			names = append(names, name)
		}
		sort.Strings(names)
		for i, name := range names {
			ref := step.Context[name]
			binding := domain.SlotBinding{ToInput: name, Ref: ref.Ref, ConstValue: ref.ConstValue, DisplayOrder: i}
			slot, exists := slotMap[name]
			if !exists {
				return nil, domain.ErrVersionNotFound
			}
			switch ref.Source {
			case "workflow_input":
				binding.Source = domain.SourceWorkflowInput
				if ref.Ref == "" || slot.ResourceType == "" {
					return nil, domain.ErrVersionNotFound
				}
				e, exists := entries[ref.Ref]
				if exists && (e.ResourceType != slot.ResourceType || e.AssetKind != slot.AssetKind) {
					return nil, domain.ErrVersionNotFound
				}
				e.Name, e.ResourceType, e.AssetKind = ref.Ref, slot.ResourceType, slot.AssetKind
				e.Required = e.Required || slot.Required
				if (slot.ResourceType == "MATERIAL" || slot.ResourceType == "ASSET") && slot.AssetKind == "" {
					return nil, domain.ErrVersionNotFound
				}
				entries[ref.Ref] = e
			case "step_output":
				binding.Source = domain.SourceStepOutput
				if ref.Ref == "" {
					return nil, domain.ErrVersionNotFound
				}
			case "const":
				binding.Source = domain.SourceConst
			default:
				return nil, domain.ErrVersionNotFound
			}
			st.SlotBindings = append(st.SlotBindings, binding)
		}
		w.Steps = append(w.Steps, st)
	}
	if !keys[w.OutputStepKey] {
		return nil, domain.ErrVersionNotFound
	}
	for _, step := range w.Steps {
		for _, binding := range step.SlotBindings {
			if binding.Source == domain.SourceStepOutput && !keys[binding.Ref] {
				return nil, domain.ErrVersionNotFound
			}
		}
	}
	for _, entry := range entries {
		w.InputSchema = append(w.InputSchema, entry)
	}
	sort.Slice(w.Steps, func(i, j int) bool { return w.Steps[i].Order < w.Steps[j].Order })
	sort.Slice(w.InputSchema, func(i, j int) bool { return w.InputSchema[i].Name < w.InputSchema[j].Name })
	return &domain.DefinitionVersion{Definition: w, Source: "compiled", Registered: true}, nil
}
