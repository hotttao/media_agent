package workflow

import (
	"context"
	"fmt"
	"testing"

	domain "media_agent/media_workflow/biz/domain/workflow"
)

// mockTemplateLookup 简单 mock：返回固定的 TemplateSnapshot。
type mockTemplateLookup struct {
	templates map[string]*domain.TemplateSnapshot // key = template_id
}

func (m *mockTemplateLookup) GetVersion(_ context.Context, id string, version int) (*domain.TemplateSnapshot, error) {
	t, ok := m.templates[id]
	if !ok {
		return nil, fmt.Errorf("template %s not found", id)
	}
	return t, nil
}

func newMockLookup() *mockTemplateLookup {
	return &mockTemplateLookup{
		templates: map[string]*domain.TemplateSnapshot{
			"tpl_a": {ID: "tpl_a", Scope: "ai_clothing", Status: "draft", OutputAssetKind: "first_frame", Platform: "gpt", GenWay: "i2i", Inputs: []domain.SlotSnapshot{{Name: "scene", ResourceType: "MATERIAL", Required: true}, {Name: "style", ResourceType: "TEXT", Required: false}}},
			"tpl_b": {ID: "tpl_b", Scope: "ai_clothing", Status: "draft", OutputAssetKind: "ai_video", Platform: "jimeng", GenWay: "i2v", Inputs: []domain.SlotSnapshot{{Name: "first_frame", ResourceType: "ASSET", Required: true}, {Name: "scene", ResourceType: "MATERIAL", Required: true}}},
		},
	}
}

func TestValidateDAG_SingleStep(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "s1", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{{ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "scene"}}},
	}, "s1", newMockLookup())
	res, _ := v.validate(context.Background())
	if !res.Valid {
		t.Fatalf("expected valid, got reasons: %v", res.RejectReasons)
	}
	if len(res.Roots) != 1 || res.Roots[0] != "s1" {
		t.Errorf("roots = %v, want [s1]", res.Roots)
	}
}

func TestValidateDAG_MultiStep(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "a", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{{ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "scene"}}},
		{Key: "b", Order: 2, TemplateID: "tpl_b", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{{ToInput: "first_frame", Source: domain.SourceStepOutput, Ref: "a"}}},
	}, "b", newMockLookup())
	res, _ := v.validate(context.Background())
	if !res.Valid {
		t.Fatalf("expected valid, got reasons: %v", res.RejectReasons)
	}
	if len(res.Roots) != 1 || res.Roots[0] != "a" {
		t.Errorf("roots = %v, want [a]", res.Roots)
	}
	if len(res.DerivedEdges) != 1 || res.DerivedEdges[0].From != "a" || res.DerivedEdges[0].To != "b" {
		t.Errorf("edges = %v", res.DerivedEdges)
	}
}

func TestValidateDAG_DuplicateStepKey(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "s1", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1},
		{Key: "s1", Order: 2, TemplateID: "tpl_b", TemplateVersion: 1},
	}, "s1", newMockLookup())
	res, _ := v.validate(context.Background())
	if res.Valid {
		t.Fatal("expected invalid for duplicate step key")
	}
	if res.RejectReasons[0].Code != "DAG_INVALID" {
		t.Errorf("code = %s, want DAG_INVALID", res.RejectReasons[0].Code)
	}
}

func TestValidateDAG_MultipleSinks(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "a", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1},
		{Key: "b", Order: 2, TemplateID: "tpl_b", TemplateVersion: 1},
	}, "a", newMockLookup())
	res, _ := v.validate(context.Background())
	if res.Valid {
		t.Fatal("expected invalid for multiple sinks")
	}
}

func TestValidateDAG_BackRef(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "a", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{{ToInput: "x", Source: domain.SourceStepOutput, Ref: "b"}}},
		{Key: "b", Order: 2, TemplateID: "tpl_b", TemplateVersion: 1},
	}, "b", newMockLookup())
	res, _ := v.validate(context.Background())
	if res.Valid {
		t.Fatal("expected invalid for back reference")
	}
}

func TestValidateDAG_OrphanStep(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "a", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1},
		{Key: "orphan", Order: 2, TemplateID: "tpl_b", TemplateVersion: 1},
	}, "a", newMockLookup())
	res, _ := v.validate(context.Background())
	if res.Valid {
		t.Fatal("expected invalid for orphan step")
	}
}

func TestValidateDAG_ScopeMismatch(t *testing.T) {
	lookup := &mockTemplateLookup{
		templates: map[string]*domain.TemplateSnapshot{
			"tpl_a": {ID: "tpl_a", Scope: "ai_guoxue", Status: "draft"}, // 不同 scope
		},
	}
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "s1", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1},
	}, "s1", lookup)
	res, _ := v.validate(context.Background())
	if res.Valid {
		t.Fatal("expected invalid for scope mismatch")
	}
	if res.RejectReasons[0].Code != "SCOPE_MISMATCH" {
		t.Errorf("code = %s, want SCOPE_MISMATCH", res.RejectReasons[0].Code)
	}
}

func TestValidateDAG_ArchivedTemplate(t *testing.T) {
	lookup := &mockTemplateLookup{
		templates: map[string]*domain.TemplateSnapshot{
			"tpl_a": {ID: "tpl_a", Scope: "ai_clothing", Status: "archived"},
		},
	}
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "s1", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1},
	}, "s1", lookup)
	res, _ := v.validate(context.Background())
	if res.Valid {
		t.Fatal("expected invalid for archived template")
	}
	if res.RejectReasons[0].Code != "TEMPLATE_ARCHIVED" {
		t.Errorf("code = %s, want TEMPLATE_ARCHIVED", res.RejectReasons[0].Code)
	}
}

func TestValidateDAG_ConstBinding(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "s1", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{
				{ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "scene"},
				{ToInput: "style", Source: domain.SourceConst, ConstValue: "cinematic"},
			}},
	}, "s1", newMockLookup())
	res, _ := v.validate(context.Background())
	if !res.Valid {
		t.Fatalf("expected valid with const binding, got: %v", res.RejectReasons)
	}
}

func TestValidateDAG_ConstEmptyValue(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "s1", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{
				{ToInput: "style", Source: domain.SourceConst, ConstValue: ""},
			}},
	}, "s1", newMockLookup())
	res, _ := v.validate(context.Background())
	if res.Valid {
		t.Fatal("expected invalid for empty const value")
	}
}

func TestDeriveInputSchema(t *testing.T) {
	v := newValidator("ai_clothing", []domain.Step{
		{Key: "a", Order: 1, TemplateID: "tpl_a", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{
				{ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "scene_input"},
				{ToInput: "style", Source: domain.SourceConst, ConstValue: "x"},
			}},
		{Key: "b", Order: 2, TemplateID: "tpl_b", TemplateVersion: 1,
			SlotBindings: []domain.SlotBinding{
				{ToInput: "scene", Source: domain.SourceWorkflowInput, Ref: "scene_input"}, // 同一个 workflow input，去重
				{ToInput: "first_frame", Source: domain.SourceStepOutput, Ref: "a"},
			}},
	}, "b", newMockLookup())
	res, _ := v.validate(context.Background())
	if !res.Valid {
		t.Fatalf("expected valid, got: %v", res.RejectReasons)
	}
	// input_schema 应只有显式入口名 "scene_input"（按 ref 去重）
	names := make(map[string]bool)
	for _, e := range res.InputSchema {
		names[e.Name] = true
	}
	if !names["scene_input"] {
		t.Errorf("input_schema missing 'scene_input': %v", res.InputSchema)
	}
	if names["ff"] {
		t.Errorf("input_schema should not contain 'ff' (step_output): %v", res.InputSchema)
	}
	if names["style"] {
		t.Errorf("input_schema should not contain 'style' (const): %v", res.InputSchema)
	}
}
