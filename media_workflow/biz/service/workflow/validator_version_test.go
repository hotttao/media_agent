package workflow

import (
	"context"
	"fmt"
	"testing"

	domain "media_agent/media_workflow/biz/domain/workflow"
)

type versionedTemplateLookup struct {
	versions map[int]*domain.TemplateSnapshot
}

func (l versionedTemplateLookup) GetVersion(_ context.Context, id string, version int) (*domain.TemplateSnapshot, error) {
	if id != "shared_template" || l.versions[version] == nil {
		return nil, fmt.Errorf("version not found")
	}
	return l.versions[version], nil
}

func TestValidatorKeepsDifferentVersionsOfSameTemplateSeparate(t *testing.T) {
	lookup := versionedTemplateLookup{versions: map[int]*domain.TemplateSnapshot{
		1: {ID: "shared_template", Scope: "ai_clothing", Status: "published", OutputAssetKind: "first_frame", Inputs: []domain.SlotSnapshot{{Name: "scene", ResourceType: "MATERIAL", AssetKind: "scene", Required: true}}},
		2: {ID: "shared_template", Scope: "ai_clothing", Status: "published", OutputAssetKind: "ai_video", Inputs: []domain.SlotSnapshot{{Name: "frame", ResourceType: "ASSET", AssetKind: "first_frame", Required: true}}},
	}}
	for _, wrongSlot := range []bool{false, true} {
		steps := []domain.Step{
			{Key: "image", Order: 1, TemplateID: "shared_template", TemplateVersion: 1, SlotBindings: []domain.SlotBinding{{ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "scene_input"}}},
			{Key: "video", Order: 2, TemplateID: "shared_template", TemplateVersion: 2, SlotBindings: []domain.SlotBinding{{ToInput: "frame", Source: domain.SourceStepOutput, Ref: "image"}}},
		}
		if wrongSlot {
			steps[0].SlotBindings[0].ToInput = "frame"
		}
		result, err := newValidator("ai_clothing", steps, "video", lookup).validate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if wrongSlot {
			if result.Valid || len(result.RejectReasons) != 1 || result.RejectReasons[0].Code != "BINDING_INVALID" {
				t.Fatal("v2 slot accepted for v1 consumer")
			}
		} else if !result.Valid || len(result.InputSchema) != 1 || result.InputSchema[0].ResourceType != "MATERIAL" || result.InputSchema[0].AssetKind != "scene" {
			t.Fatalf("versions contaminated validation: %#v", result)
		}
	}
}

func TestSharedWorkflowInputRequiredIfAnyConsumerRequiresIt(t *testing.T) {
	for _, required := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprint(required), func(t *testing.T) {
			lookup := newMockLookup()
			lookup.templates["tpl_a"].Inputs[0].Required = required[0]
			lookup.templates["tpl_b"].Inputs[1].Required = required[1]
			steps := []domain.Step{
				{Key: "image", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1, SlotBindings: []domain.SlotBinding{{ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "shared"}}},
				{Key: "video", Order: 2, TemplateID: "tpl_b", TemplateVersion: 1, SlotBindings: []domain.SlotBinding{{ToInput: "first_frame", Source: domain.SourceStepOutput, Ref: "image"}, {ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "shared"}}},
			}
			result, err := newValidator("ai_clothing", steps, "video", lookup).validate(context.Background())
			if err != nil || !result.Valid || len(result.InputSchema) != 1 || result.InputSchema[0].Required != (required[0] || required[1]) {
				t.Fatalf("mandatory consumer weakened: %#v error=%v", result, err)
			}
		})
	}
}
