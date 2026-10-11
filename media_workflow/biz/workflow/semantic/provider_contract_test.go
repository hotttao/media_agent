package semantic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	capability "media_agent/media_workflow/biz/domain/capability"
	template "media_agent/media_workflow/biz/domain/template"
	workflow "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/compiler"
	"media_agent/media_workflow/biz/workflow/executable"
)

func providerCompileFixture() (workflow.Workflow, Snapshots) {
	wf := workflow.Workflow{ID: "provider_reuse", Version: 1, OutputStepKey: "style", Steps: []workflow.Step{
		{Key: "model", Order: 1, TemplateID: "model_tpl", TemplateVersion: 1, SlotBindings: []workflow.SlotBinding{{ToInput: "person", Source: workflow.SourceWorkflowInput, Ref: "person"}}},
		{Key: "style", Order: 2, TemplateID: "style_tpl", TemplateVersion: 1, SlotBindings: []workflow.SlotBinding{{ToInput: "person", Source: workflow.SourceStepOutput, Ref: "model"}}},
	}}
	key := "jimeng.i2i.multi_image_prompt"
	s := Snapshots{Templates: map[TemplateRef]TemplateSnapshot{}, Capabilities: map[CapabilityRef]CapabilitySnapshot{
		{Key: key, Version: 1}: {Key: key, Version: 1, Provider: "jimeng", GenerationMethod: capability.ImageToImage, GenerationType: capability.MultiImagePrompt,
			ConductorTaskDef: "jimeng_i2i_multi_image_v1", ExecutionTarget: capability.ExecutionTargetPersonalWorker, OutputMediaType: capability.MediaImage,
			Inputs: []CapabilityInput{{Name: "prompt", Type: TypeString, Required: true}, {Name: "images", Type: TypeResourceList, Required: true, MediaType: capability.MediaImage, MinItems: 1}}},
	}}
	for _, name := range []string{"model", "style"} {
		s.Templates[TemplateRef{ID: name + "_tpl", Version: 1}] = TemplateSnapshot{ID: name + "_tpl", Version: 1, Body: "{{.person.description}}",
			Inputs:          []TemplateSlot{{Name: "person", Resource: true, Required: true, MediaType: capability.MediaImage}},
			OutputAssetKind: name + "_image", OutputMediaType: capability.MediaImage, CapabilityKey: key, CapabilityVersion: 1, CandidateCount: 1,
			Invocation: template.InvocationMapping{"prompt": {Op: template.InvocationRenderTemplate}, "images": {Op: template.InvocationList, Items: []template.InvocationExpression{{Op: template.InvocationAssetRef, Slot: "person"}}}}}
	}
	return wf, s
}

func TestProviderCapabilityReuseKeepsBusinessKindsAndFreezesContract(t *testing.T) {
	wf, snapshots := providerCompileFixture()
	ir, err := Compile(wf, snapshots)
	require.NoError(t, err)
	require.Equal(t, "model_image", ir.Steps[0].OutputAssetKind)
	require.Equal(t, "style_image", ir.Steps[1].OutputAssetKind)
	require.Equal(t, ir.Steps[0].Capability, ir.Steps[1].Capability)
	require.Equal(t, "IMAGE", ir.Steps[0].Capability.OutputMediaType)
	require.Equal(t, "multi_image_prompt", ir.Steps[0].Capability.GenerationType)
	require.Equal(t, capability.MediaImage, ir.Steps[0].Capability.Inputs[1].MediaType)
	require.Equal(t, 1, ir.Steps[0].Capability.Inputs[1].MinItems)
	before, err := executable.CanonicalJSON(ir)
	require.NoError(t, err)
	ref := CapabilityRef{Key: "jimeng.i2i.multi_image_prompt", Version: 1}
	changed := snapshots.Capabilities[ref]
	changed.Inputs[1].MinItems = 5
	snapshots.Capabilities[ref] = changed
	after, err := executable.CanonicalJSON(ir)
	require.NoError(t, err)
	require.Equal(t, before, after, "frozen contract must not alias mutable registry snapshots")
	definition, err := compiler.CompileExecutableIR(ir)
	require.NoError(t, err)
	encoded, err := json.Marshal(definition)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "jimeng_i2i_multi_image_v1")
	require.Contains(t, string(encoded), "model_image")
	require.Contains(t, string(encoded), "style_image")
}

func TestProviderCompileRejectsIncompatibleOutputInputsAndCounts(t *testing.T) {
	for _, scenario := range []string{"video_output", "unknown_output", "video_input", "unknown_input", "first_frame_two_images", "negative_limit"} {
		t.Run(scenario, func(t *testing.T) {
			wf, s := providerCompileFixture()
			tref := TemplateRef{ID: "model_tpl", Version: 1}
			cref := CapabilityRef{Key: "jimeng.i2i.multi_image_prompt", Version: 1}
			tpl, cap := s.Templates[tref], s.Capabilities[cref]
			switch scenario {
			case "video_output":
				tpl.OutputMediaType = capability.MediaVideo
			case "unknown_output":
				tpl.OutputMediaType = ""
			case "video_input":
				tpl.Inputs[0].MediaType = capability.MediaVideo
			case "unknown_input":
				tpl.Inputs[0].MediaType = ""
			case "first_frame_two_images":
				cap.Inputs[1].MaxItems = 1
				expr := tpl.Invocation["images"]
				expr.Items = append(expr.Items, expr.Items[0])
				tpl.Invocation["images"] = expr
			case "negative_limit":
				cap.Inputs[1].MinItems = -1
			}
			s.Templates[tref], s.Capabilities[cref] = tpl, cap
			_, err := Compile(wf, s)
			require.Error(t, err)
		})
	}
}

func TestLegacyCapabilityCanonicalBytesRemainUnchanged(t *testing.T) {
	const legacy = `{"schema_version":3,"workflow_id":"old","workflow_version":1,"output_step_key":"s","steps":[{"key":"s","order":1,"template":{"id":"tpl","version":1,"body":"prompt"},"context":{},"capability":{"key":"media.image.model","version":1,"conductor_task_def":"model_v1","execution_target":"personal_worker"},"invocation":{},"output_asset_kind":"model_image","approval_policy":"","approval_required":false,"candidate_count":1,"reuse_policy":""}]}`
	var ir executable.ExecutableWorkflowIR
	require.NoError(t, json.Unmarshal([]byte(legacy), &ir))
	actual, err := executable.CanonicalJSON(ir)
	require.NoError(t, err)
	require.Equal(t, legacy, string(actual))
}

func TestFreshCompilePinsLegacyInputsWithoutInventingProviderContract(t *testing.T) {
	wf, s := providerCompileFixture()
	ref := CapabilityRef{Key: "jimeng.i2i.multi_image_prompt", Version: 1}
	cap := s.Capabilities[ref]
	cap.Provider = "client"
	cap.GenerationMethod, cap.GenerationType, cap.OutputMediaType = "", "", ""
	cap.OutputAssetKind = "model_image"
	cap.Inputs[1].MediaType, cap.Inputs[1].MinItems = "", 0
	s.Capabilities[ref] = cap
	tref := TemplateRef{ID: "style_tpl", Version: 1}
	tpl := s.Templates[tref]
	tpl.OutputAssetKind = "model_image"
	s.Templates[tref] = tpl
	ir, err := Compile(wf, s)
	require.NoError(t, err)
	encoded, err := executable.CanonicalJSON(ir)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"provider"`)
	require.NotContains(t, string(encoded), `"output_media_type"`)
	require.NotContains(t, string(encoded), `"generation_method"`)
	require.Equal(t, executable.SchemaVersion, ir.SchemaVersion)
	require.Len(t, ir.Steps[0].Capability.Inputs, len(cap.Inputs))
	for index, expected := range cap.Inputs {
		actual := ir.Steps[0].Capability.Inputs[index]
		require.Equal(t, expected.Name, actual.Name)
		require.Equal(t, string(expected.Type), string(actual.Type))
		require.Equal(t, expected.Required, actual.Required)
		require.Equal(t, string(expected.MediaType), string(actual.MediaType))
		require.Equal(t, expected.MinItems, actual.MinItems)
		require.Equal(t, expected.MaxItems, actual.MaxItems)
	}
}
