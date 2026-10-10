//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	repo "media_agent/media_workflow/biz/dal/repo/workflow"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/workflow/executable"
)

// Isolated real PG: no deployed service, engine, user resource or Worker.
func TestWorkflowBusinessVersionPersistence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	downgrade, err := os.ReadFile("../migrations/000026_workflow_versions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(downgrade)).Error; err != nil {
		t.Fatal("empty downgrade", err)
	}
	now := time.Now()
	if err := db.Create(&model.Template{ID: "frozen_tpl", Scope: "version_test", Name: "template", PromptBody: "test", OutputAssetKind: "first_frame", Version: 1, CreatedBy: "owner", CreatedAt: now, UpdatedAt: now, InvocationMapping: []byte(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Workflow{ID: "wf_version_test", Scope: "version_test", Name: "first", OutputStepKey: "result", Status: "draft", Visibility: "private", Version: 1, CreatedBy: "owner", CreatedAt: now, UpdatedAt: now, InputSchema: []byte(`[{"name":"text","resource_type":"TEXT","required":true}]`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorkflowStep{ID: "step_before_migration", WorkflowID: "wf_version_test", Version: 1, StepKey: "result", StepName: "first result", StepOrder: 0, TemplateID: "frozen_tpl", TemplateVersion: 1, CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	ref := "text"
	if err := db.Create(&model.StepBinding{ID: "binding_before_migration", StepID: "step_before_migration", ToInput: "text", Source: "WORKFLOW_INPUT", Ref: &ref, DisplayOrder: 0, CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	upgrade, err := os.ReadFile("../migrations/000026_workflow_versions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(upgrade)).Error; err != nil {
		t.Fatal("backfill", err)
	}
	r := repo.NewRepo(db)
	first, err := r.GetDefinitionVersion(ctx, "wf_version_test", 1)
	if err != nil || first.Source != "snapshot" || first.Definition.Name != "first" || first.Definition.Steps[0].SlotBindings[0].Ref != "text" || !first.Definition.InputSchema[0].Required {
		t.Fatal("backfill lost actual fields", err)
	}
	tx := dal.NewTxRunner(db)
	w, err := r.Get(ctx, "wf_version_test")
	if err != nil {
		t.Fatal(err)
	}
	w.Version = 2
	w.Name = "second"
	w.Steps[0].Name = "second result"
	rollback := errors.New("injected after persistence")
	err = tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := r.Update(ctx, w); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("update failed before rollback boundary", err)
	}
	current, err := r.Get(ctx, w.ID)
	if err != nil || current.Version != 1 || current.Name != "first" {
		t.Fatal("head did not rollback", err)
	}
	if _, err := r.GetDefinitionVersion(ctx, w.ID, 2); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Fatal("snapshot escaped rollback", err)
	}
	if err := tx.RunInTx(ctx, func(ctx context.Context) error { return r.Update(ctx, w) }); err != nil {
		t.Fatal("update", err)
	}
	second, err := r.GetDefinitionVersion(ctx, w.ID, 2)
	if err != nil || second.Definition.Name != "second" || second.Definition.Steps[0].Name != "second result" {
		t.Fatal("new snapshot", err)
	}
	first, err = r.GetDefinitionVersion(ctx, w.ID, 1)
	if err != nil || first.Definition.Name != "first" || first.Definition.Steps[0].Name != "first result" {
		t.Fatal("old snapshot changed", err)
	}
	if err := db.Model(&model.WorkflowVersion{}).Where("workflow_id=? AND version=1", w.ID).Update("definition_snapshot", []byte(`{}`)).Error; err == nil {
		t.Fatal("database overwrote frozen definition")
	}
	if err := db.Exec(string(downgrade)).Error; err == nil {
		t.Fatal("non-empty downgrade destroyed history")
	}
	if err := tx.RunInTx(ctx, func(ctx context.Context) error { return r.Update(ctx, w) }); !errors.Is(err, domain.ErrPublicationConflict) {
		t.Fatal("stale version overwritten", err)
	}
	created := &domain.Workflow{ID: "new_version_test", Scope: "version_test", Name: "created", Version: 1, Status: domain.StatusDraft, Visibility: domain.VisibilityPrivate, CreatedBy: "owner", OutputStepKey: "result", Steps: []domain.Step{{Key: "result", TemplateID: "frozen_tpl", TemplateVersion: 1}}}
	if err := tx.RunInTx(ctx, func(ctx context.Context) error { return r.Save(ctx, created) }); err != nil {
		t.Fatal("create snapshot", err)
	}
	if _, err := r.GetDefinitionVersion(ctx, created.ID, 1); err != nil {
		t.Fatal("new workflow lacks version", err)
	}
	var stored model.WorkflowVersion
	if err := db.Where("workflow_id=? AND version=1", created.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if json.Unmarshal(stored.DefinitionSnapshot, &fields) != nil {
		t.Fatal("invalid snapshot JSON")
	}
	for _, key := range []string{"visibility", "created_by", "organization_id", "workspace_id", "publication_id", "status"} {
		if _, exists := fields[key]; exists {
			t.Fatal("mutable facts frozen", key)
		}
	}
}

func TestWorkflowCompiledVersionEvidence(t *testing.T) {
	db := newDatabase(t)
	ctx := context.Background()
	now := time.Now()
	if err := db.Create(&model.Template{ID: "evidence_tpl", Scope: "evidence", Name: "current template", PromptBody: "current", OutputAssetKind: "first_frame", Version: 2, CreatedBy: "owner", CreatedAt: now, UpdatedAt: now, InvocationMapping: []byte(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.TemplateVersion{ID: "evidence_tpl_v1", TemplateID: "evidence_tpl", Version: 1, Status: "published", PromptBody: "frozen", OutputAssetKind: "first_frame", InputsSnapshot: []byte(`[{"Name":"source","ResourceType":"MATERIAL","AssetKind":"movement","Required":true}]`), InvocationMapping: []byte(`{}`), CreatedBy: "owner", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Workflow{ID: "evidence_wf", Scope: "evidence", Name: "current workflow", OutputStepKey: "new_output", Version: 2, Status: "draft", Visibility: "private", InputSchema: []byte(`[]`), CreatedBy: "owner", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	base := executable.ExecutableWorkflowIR{SchemaVersion: 3, WorkflowID: "evidence_wf", WorkflowVersion: 1, OutputStepKey: "old_output", Steps: []executable.ExecutableStepIR{
		{Key: "old_output", Order: 1, Template: executable.TemplateIR{ID: "evidence_tpl", Version: 1}, Context: map[string]executable.ValueRef{"source": {Source: executable.ValueWorkflowInput, Ref: "old_input"}}, ApprovalPolicy: "always", ReusePolicy: "force_new"},
	}}
	insert := func(id string, version int, ir executable.ExecutableWorkflowIR, status string) {
		t.Helper()
		ir.WorkflowVersion = version
		raw, err := json.Marshal(ir)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.WorkflowCompilation{ID: id, WorkflowID: "evidence_wf", WorkflowVersion: version, ConductorName: id, ConductorVersion: 1, IRJSON: raw, DefinitionJSON: []byte(`{}`), DefinitionHash: id, Status: status, CreatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
	}
	insert("valid_evidence", 1, base, "registered")
	r := repo.NewRepo(db)
	got, err := r.GetDefinitionVersion(ctx, "evidence_wf", 1)
	if err != nil || got.Source != "compiled" || !got.Registered || got.Definition.OutputStepKey != "old_output" || got.Definition.Name != "" || got.Definition.Scope != "" || got.Definition.Steps[0].ApprovalPolicy != domain.ApprovalAlways || got.Definition.Steps[0].TemplateVersion != 1 || len(got.Definition.InputSchema) != 1 || got.Definition.InputSchema[0].AssetKind != "movement" || !got.Definition.InputSchema[0].Required {
		t.Fatal("frozen evidence drift", err)
	}
	insert("unregistered_evidence", 3, base, "failed")
	if _, err := r.GetDefinitionVersion(ctx, "evidence_wf", 3); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Fatal("failed compilation served", err)
	}
	base.Steps[0].Template.Version = 99
	insert("missing_template_evidence", 4, base, "registered")
	if _, err := r.GetDefinitionVersion(ctx, "evidence_wf", 4); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Fatal("missing frozen template replaced by current", err)
	}
	base.Steps[0].Template.Version = 1
	base.Steps[0].Context["source"] = executable.ValueRef{Source: "unknown", Ref: "old_input"}
	insert("invalid_source_evidence", 5, base, "registered")
	if _, err := r.GetDefinitionVersion(ctx, "evidence_wf", 5); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Fatal("unknown binding inferred", err)
	}
	if _, err := r.GetDefinitionVersion(ctx, "evidence_wf", 6); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Fatal("lost draft invented", err)
	}
	var snapshots int64
	if err := db.Model(&model.WorkflowVersion{}).Where("workflow_id='evidence_wf'").Count(&snapshots).Error; err != nil || snapshots != 0 {
		t.Fatal("read reconstructed or wrote immutable history", err)
	}
}
