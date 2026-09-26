package semantic

import (
	"testing"

	"github.com/stretchr/testify/require"
	template "media_agent/media_workflow/biz/domain/template"
	workflow "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

func TestCompileBuildsTypedExecutableWorkflowIR(t *testing.T) {
	wf := workflow.Workflow{
		ID: "clothing_video", Version: 1, OutputStepKey: "model_image",
		Steps: []workflow.Step{{
			Key: "model_image", Order: 1, TemplateID: "tpl_model", TemplateVersion: 3,
			ApprovalPolicy: workflow.ApprovalWhenMultipleCandidates, ReusePolicy: workflow.ReuseIfAvailable,
			SlotBindings: []workflow.SlotBinding{
				{ToInput: "person", Source: workflow.SourceWorkflowInput, Ref: "full_body"},
				{ToInput: "garment", Source: workflow.SourceWorkflowInput, Ref: "clothing"},
			},
		}},
	}
	mapping := template.InvocationMapping{
		"prompt": {Op: template.InvocationRenderTemplate},
		"images": {Op: template.InvocationList, Items: []template.InvocationExpression{
			{Op: template.InvocationAssetRef, Slot: "person"},
			{Op: template.InvocationAssetRef, Slot: "garment"},
		}},
	}
	snapshots := Snapshots{
		Templates: map[TemplateRef]TemplateSnapshot{
			{ID: "tpl_model", Version: 3}: {
				ID: "tpl_model", Version: 3, Body: "{{.person.description}} wears {{.garment.name}}",
				Inputs:          []TemplateSlot{{Name: "person", Resource: true, Required: true}, {Name: "garment", Resource: true, Required: true}},
				OutputAssetKind: "model_image", CapabilityKey: "jimeng.image.multi", CapabilityVersion: 1, Invocation: mapping,
				CandidateCount: 2,
			},
		},
		Capabilities: map[CapabilityRef]CapabilitySnapshot{
			{Key: "jimeng.image.multi", Version: 1}: {
				Key: "jimeng.image.multi", Version: 1, ConductorTaskDef: "jimeng_image_multi_v1", OutputAssetKind: "model_image",
				Inputs: []CapabilityInput{{Name: "prompt", Type: TypeString, Required: true}, {Name: "images", Type: TypeResourceList, Required: true}},
			},
		},
	}

	ir, err := Compile(wf, snapshots)
	require.NoError(t, err)
	require.Equal(t, executable.SchemaVersion, ir.SchemaVersion)
	require.Len(t, ir.Steps, 1)
	require.Equal(t, "full_body", ir.Steps[0].Context["person"].Ref)
	require.Equal(t, "clothing", ir.Steps[0].Context["garment"].Ref)
	require.Equal(t, "jimeng_image_multi_v1", ir.Steps[0].Capability.ConductorTaskDef)
	require.Equal(t, "person", ir.Steps[0].Invocation["images"].Items[0].Slot)
	require.Equal(t, "garment", ir.Steps[0].Invocation["images"].Items[1].Slot)
	require.True(t, ir.Steps[0].ApprovalRequired)
	require.Equal(t, 2, ir.Steps[0].CandidateCount)
}

func TestCompileRejectsMissingAndMistypedCapabilityInputs(t *testing.T) {
	wf := workflow.Workflow{ID: "wf", Version: 1, OutputStepKey: "s", Steps: []workflow.Step{{
		Key: "s", Order: 1, TemplateID: "tpl", TemplateVersion: 1,
		SlotBindings: []workflow.SlotBinding{{ToInput: "text", Source: workflow.SourceWorkflowInput, Ref: "description"}},
	}}}
	tpl := TemplateSnapshot{
		ID: "tpl", Version: 1, Inputs: []TemplateSlot{{Name: "text", Required: true}},
		OutputAssetKind: "image", CapabilityKey: "cap", CapabilityVersion: 1,
		Invocation: template.InvocationMapping{"images": {Op: template.InvocationRenderTemplate}},
	}
	capability := CapabilitySnapshot{
		Key: "cap", Version: 1, ConductorTaskDef: "cap_v1", OutputAssetKind: "image",
		Inputs: []CapabilityInput{{Name: "prompt", Type: TypeString, Required: true}, {Name: "images", Type: TypeResourceList, Required: true}},
	}
	_, err := Compile(wf, Snapshots{
		Templates:    map[TemplateRef]TemplateSnapshot{{ID: "tpl", Version: 1}: tpl},
		Capabilities: map[CapabilityRef]CapabilitySnapshot{{Key: "cap", Version: 1}: capability},
	})
	require.ErrorContains(t, err, "required capability input \"prompt\" is not mapped")

	tpl.Invocation["prompt"] = template.InvocationExpression{Op: template.InvocationRenderTemplate}
	_, err = Compile(wf, Snapshots{
		Templates:    map[TemplateRef]TemplateSnapshot{{ID: "tpl", Version: 1}: tpl},
		Capabilities: map[CapabilityRef]CapabilitySnapshot{{Key: "cap", Version: 1}: capability},
	})
	require.ErrorContains(t, err, "input \"images\" expects resource_ref_list, got string")
}

func TestCompileRequiresExplicitWorkflowInputReference(t *testing.T) {
	wf := workflow.Workflow{ID: "wf", Version: 1, OutputStepKey: "s", Steps: []workflow.Step{{
		Key: "s", Order: 1, TemplateID: "tpl", TemplateVersion: 1,
		SlotBindings: []workflow.SlotBinding{{ToInput: "person", Source: workflow.SourceWorkflowInput}},
	}}}
	_, err := Compile(wf, Snapshots{Templates: map[TemplateRef]TemplateSnapshot{
		{ID: "tpl", Version: 1}: {ID: "tpl", Version: 1, Inputs: []TemplateSlot{{Name: "person", Required: true}}, CapabilityKey: "cap", CapabilityVersion: 1},
	}, Capabilities: map[CapabilityRef]CapabilitySnapshot{
		{Key: "cap", Version: 1}: {Key: "cap", Version: 1, ConductorTaskDef: "cap_v1"},
	}})
	require.ErrorContains(t, err, "requires explicit ref")
}
