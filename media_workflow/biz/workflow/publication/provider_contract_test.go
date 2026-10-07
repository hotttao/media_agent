package publication

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	capability "media_agent/media_workflow/biz/domain/capability"
	workflow "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared"
	"media_agent/media_workflow/biz/workflow/executable"
)

type providerTemplates struct{ workflow.TemplateLookup }

func (providerTemplates) GetVersion(context.Context, string, int) (*workflow.TemplateSnapshot, error) {
	return &workflow.TemplateSnapshot{ID: "tpl", PromptBody: "{{.person.description}}", OutputAssetKind: "model_image", CandidateCount: 1,
		CapabilityKey: "jimeng.i2i.multi_image_prompt", CapabilityVersion: 1,
		Inputs:            []workflow.SlotSnapshot{{Name: "person", AssetKind: "person_image", ResourceType: "MATERIAL", Required: true}, {Name: "garment", AssetKind: "garment_image", ResourceType: "MATERIAL", Required: true}},
		InvocationMapping: map[string]workflow.InvocationExpressionSnapshot{"prompt": {Op: "render_template"}, "images": {Op: "list", Items: []workflow.InvocationExpressionSnapshot{{Op: "asset_ref", Slot: "person"}, {Op: "asset_ref", Slot: "garment"}}}}}, nil
}

type providerRegistry struct{ capability.Repo }

type providerAggregateTemplates struct{ providerTemplates }

func (r providerAggregateTemplates) GetVersion(ctx context.Context, id string, version int) (*workflow.TemplateSnapshot, error) {
	tpl, err := r.providerTemplates.GetVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	tpl.Inputs[0].AssetKind, tpl.Inputs[0].ResourceType = "virtual_ip", "VIRTUAL_IP"
	tpl.Inputs[1].AssetKind, tpl.Inputs[1].ResourceType = "product", "PRODUCT"
	return tpl, nil
}

func TestPublicationAggregateImageSlotsAreNotAssetKinds(t *testing.T) {
	wf := &workflow.Workflow{ID: "aggregates", Version: 1, OutputStepKey: "model", Steps: []workflow.Step{{Key: "model", Order: 1, TemplateID: "tpl", TemplateVersion: 1, SlotBindings: []workflow.SlotBinding{{ToInput: "person", Source: workflow.SourceWorkflowInput, Ref: "virtual_ip"}, {ToInput: "garment", Source: workflow.SourceWorkflowInput, Ref: "product"}}}}}
	flow := &Flow{Templates: providerAggregateTemplates{}, Capabilities: providerRegistry{}, AssetKinds: providerKinds{values: map[string]*shared.AssetKindInfo{"model_image": {ResourceType: "ASSET", MediaType: "IMAGE", Status: "SYSTEM"}}}}
	if _, err := flow.compile(context.Background(), wf); err != nil {
		t.Fatal("aggregate image input incorrectly required registry entry", err)
	}
}

func (providerRegistry) Get(context.Context, string, int) (*capability.Capability, error) {
	return &capability.Capability{Key: "jimeng.i2i.multi_image_prompt", Version: 1, Provider: "jimeng", GenerationMethod: capability.ImageToImage, GenerationType: capability.MultiImagePrompt,
		OutputMediaType: capability.MediaImage, ExecutionTarget: capability.ExecutionTargetPersonalWorker, ConductorTaskDef: "jimeng_i2i_multi_image_v1",
		Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}, {Name: "images", Type: capability.InputResourceRefList, Required: true, MediaType: capability.MediaImage, MinItems: 1}}}, nil
}

type providerKinds struct {
	values map[string]*shared.AssetKindInfo
	err    error
}

func (k providerKinds) BatchGet(context.Context, []string) (map[string]*shared.AssetKindInfo, []string, error) {
	return k.values, nil, k.err
}

func TestPublicationFreezesProviderMediaConstraintsAndImageOrder(t *testing.T) {
	wf := &workflow.Workflow{ID: "provider", Version: 1, OutputStepKey: "model", Steps: []workflow.Step{{Key: "model", Order: 1, TemplateID: "tpl", TemplateVersion: 1,
		SlotBindings: []workflow.SlotBinding{{ToInput: "person", Source: workflow.SourceWorkflowInput, Ref: "person"}, {ToInput: "garment", Source: workflow.SourceWorkflowInput, Ref: "garment"}}}}}
	kinds := providerKinds{values: map[string]*shared.AssetKindInfo{
		"model_image":   {ResourceType: "ASSET", MediaType: "IMAGE", Status: "SYSTEM"},
		"person_image":  {ResourceType: "MATERIAL", MediaType: "IMAGE", Status: "SYSTEM"},
		"garment_image": {ResourceType: "MATERIAL", MediaType: "IMAGE", Status: "ENABLED"},
	}}
	flow := &Flow{Templates: providerTemplates{}, Capabilities: providerRegistry{}, AssetKinds: kinds}
	prepared, err := flow.compile(context.Background(), wf)
	require.NoError(t, err)
	var ir executable.ExecutableWorkflowIR
	require.NoError(t, json.Unmarshal(prepared.compilation.IRJSON, &ir))
	require.Equal(t, "jimeng", ir.Steps[0].Capability.Provider)
	require.Equal(t, "i2i", ir.Steps[0].Capability.GenerationMethod)
	require.Equal(t, capability.MediaImage, ir.Steps[0].Capability.Inputs[1].MediaType)
	require.Equal(t, 1, ir.Steps[0].Capability.Inputs[1].MinItems)
	require.Equal(t, "person", ir.Steps[0].Invocation["images"].Items[0].Slot)
	require.Equal(t, "garment", ir.Steps[0].Invocation["images"].Items[1].Slot)
	require.Equal(t, "model_image", ir.Steps[0].OutputAssetKind)
	for _, scenario := range []string{"missing", "unknown_media", "video_input", "disabled", "wrong_resource_type", "metadata_offline", "no_lookup"} {
		t.Run(scenario, func(t *testing.T) {
			bad := providerKinds{values: map[string]*shared.AssetKindInfo{}}
			for name, info := range kinds.values {
				copy := *info
				bad.values[name] = &copy
			}
			switch scenario {
			case "missing":
				delete(bad.values, "person_image")
			case "unknown_media":
				bad.values["model_image"].MediaType = ""
			case "video_input":
				bad.values["person_image"].MediaType = "VIDEO"
			case "disabled":
				bad.values["garment_image"].Status = "DISABLED"
			case "wrong_resource_type":
				bad.values["model_image"].ResourceType = "MATERIAL"
			case "metadata_offline":
				bad.err = errors.New("offline")
			}
			flow.AssetKinds = bad
			if scenario == "no_lookup" {
				flow.AssetKinds = nil
			}
			_, err := flow.compile(context.Background(), wf)
			require.Error(t, err)
		})
	}
}
