package job

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	domain "media_agent/media_workflow/biz/domain/job"
	wfdomain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared/client"
	"media_agent/media_workflow/biz/workflow/executable"
)

type historyCompilationRepo struct{ ir []byte }

func (r historyCompilationRepo) Save(context.Context, *wfdomain.Compilation) error { return nil }
func (r historyCompilationRepo) MarkRegistered(context.Context, string) error      { return nil }
func (r historyCompilationRepo) MarkFailed(context.Context, string, string) error  { return nil }
func (r historyCompilationRepo) GetRegistered(context.Context, string, int) (*wfdomain.Compilation, error) {
	return &wfdomain.Compilation{IRJSON: r.ir, Status: "registered"}, nil
}

type historyAssetLookup struct {
	assets map[string]*client.ImportedGeneratedAsset
}

type historyWorkspaceLookup struct {
	workspace *client.WorkspaceContext
}

func (m historyWorkspaceLookup) GetWorkspaceForUser(_ context.Context, workspaceID, userID string) (*client.WorkspaceContext, error) {
	if m.workspace != nil && m.workspace.ID == workspaceID && userID == "owner" {
		return m.workspace, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (m historyAssetLookup) GetImportedGeneratedAsset(_ context.Context, id string) (*client.ImportedGeneratedAsset, error) {
	if a := m.assets[id]; a != nil {
		return a, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func historyImportFixture(t *testing.T) (*Service, *mockJobRepo, ImportIncompleteCommand) {
	t.Helper()
	keys := []string{"model_image", "style_image", "first_frame", "video"}
	kinds := []string{"model_image", "style_image", "first_frame", "ai_video"}
	ir := executable.ExecutableWorkflowIR{SchemaVersion: executable.SchemaVersion, WorkflowID: "wf", WorkflowVersion: 1, OutputStepKey: "video"}
	for i, key := range keys {
		step := executable.ExecutableStepIR{Key: key, Order: i + 1, Template: executable.TemplateIR{ID: "tpl-" + key, Version: 1}, OutputAssetKind: kinds[i]}
		if i > 0 {
			step.DependsOn = []string{keys[i-1]}
		}
		ir.Steps = append(ir.Steps, step)
	}
	encoded, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockJobRepo{jobs: map[string]*domain.Job{}, stepRuns: map[string][]*domain.StepRun{}}
	svc := NewService(repo, &mockStepRunRepo{items: map[string]*domain.StepRun{}}, nil,
		&mockWorkflowLookup{wf: &wfdomain.Workflow{ID: "wf", Scope: "ai_clothing", Status: wfdomain.StatusPublished, Version: 1}},
		&mockTemplateLookup{}, &mockMaterialsClient{}, nil, historyCompilationRepo{ir: encoded}, nil)
	assets := map[string]*client.ImportedGeneratedAsset{}
	cmd := ImportIncompleteCommand{SourceGroupID: "frame:frame-source", OrganizationID: "org", WorkspaceID: "workspace", OwnerUserID: "owner",
		WorkflowID: "wf", WorkflowVersion: 1, Input: domain.JobInput{Product: &domain.ProductData{ID: "product"}, VirtualIP: &domain.VirtualIpData{ID: "vip"}}}
	for i := 0; i < 3; i++ {
		id, sourceID := "gen-"+keys[i], keys[i]+"-source"
		if i == 2 {
			sourceID = "frame-source"
		}
		cmd.Completed = append(cmd.Completed, HistoricalStep{StepKey: keys[i], SourceRecordID: sourceID, AssetID: id})
		assets[id] = &client.ImportedGeneratedAsset{ID: id, OrganizationID: "org", Scope: "ai_clothing", OwnerUserID: "owner",
			AssetKind: kinds[i], ObjectPath: "materials/org/ai_clothing/imported/" + id, SourceSystem: "media_ai", SourceRecordID: sourceID,
			SourceSHA256: strings.Repeat("a", 64), SourceCreatedAt: time.Now().UnixNano(), SourceProductID: "product", SourceVirtualIPID: "vip"}
	}
	svc.ConfigureHistoryAssets(historyAssetLookup{assets: assets})
	svc.ConfigureHistoryWorkspaces(historyWorkspaceLookup{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "org", TeamID: "team"}})
	return svc, repo, cmd
}

func TestImportIncompleteFreezesPrefixAndIsIdempotent(t *testing.T) {
	svc, repo, cmd := historyImportFixture(t)
	first, err := svc.ImportIncomplete(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ImportIncomplete(context.Background(), cmd)
	if err != nil || first.ID != second.ID || len(repo.jobs) != 1 {
		t.Fatalf("import was not idempotent: first=%+v second=%+v err=%v", first, second, err)
	}
	if first.Status != domain.JobAwaitingResume || first.ConductorWorkflowID != "" || first.Origin != "legacy_import" {
		t.Fatalf("historical Job unexpectedly started: %+v", first)
	}
	steps := repo.stepRuns[first.ID]
	if len(steps) != 4 || steps[0].Status != domain.SRSucceeded || steps[1].Status != domain.SRSucceeded ||
		steps[2].Status != domain.SRSucceeded || steps[3].Status != domain.SRBlocked || steps[2].OutputAssetID == "" || steps[3].OutputAssetID != "" {
		t.Fatalf("incorrect historical prefix: %+v", steps)
	}
	cmd.Completed[2].AssetID = "another"
	if _, err := svc.ImportIncomplete(context.Background(), cmd); !errors.Is(err, ErrHistoryImportConflict) {
		t.Fatalf("changed source was not rejected: %v", err)
	}
}

func TestImportCompletedVideoIsTerminalIdempotentAndDoesNotStartConductor(t *testing.T) {
	svc, repo, cmd := historyImportFixture(t)
	videoCreated := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	cmd.SourceGroupID = "video:video-source"
	cmd.Completed = append(cmd.Completed, HistoricalStep{StepKey: "video", SourceRecordID: "video-source", AssetID: "gen-video"})
	lookup := svc.historyAssets.(historyAssetLookup)
	lookup.assets["gen-video"] = &client.ImportedGeneratedAsset{
		ID: "gen-video", OrganizationID: "org", Scope: "ai_clothing", OwnerUserID: "owner",
		AssetKind: "ai_video", ObjectPath: "materials/org/ai_clothing/imported/gen-video",
		SourceSystem: "media_ai", SourceRecordID: "video-source", SourceSHA256: strings.Repeat("b", 64),
		SourceCreatedAt: videoCreated.UnixNano(), SourceProductID: "product", SourceVirtualIPID: "vip",
	}
	job, err := svc.ImportCompleted(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := svc.ImportCompleted(context.Background(), cmd)
	if err != nil || repeated.ID != job.ID || len(repo.jobs) != 1 {
		t.Fatalf("completed video import not idempotent: job=%+v repeated=%+v err=%v", job, repeated, err)
	}
	if job.Status != domain.JobSucceeded || job.ConductorWorkflowID != "" || job.FinishedAt == nil || !job.FinishedAt.Equal(videoCreated) || job.Origin != "legacy_import" {
		t.Fatalf("completed historical video was represented as a live execution: %+v", job)
	}
	steps := repo.stepRuns[job.ID]
	if len(steps) != 4 {
		t.Fatalf("wanted 4 frozen historical steps, got %d", len(steps))
	}
	for _, step := range steps {
		if step.Status != domain.SRSucceeded || step.Origin != "legacy_import" || step.OutputAssetID == "" {
			t.Fatalf("historical prefix has an unfinished step: %+v", step)
		}
	}
	cmd.Completed[3].AssetID = "another-video-asset"
	if _, err := svc.ImportCompleted(context.Background(), cmd); !errors.Is(err, ErrHistoryImportConflict) {
		t.Fatalf("changed video source not rejected: %v", err)
	}
}

func TestImportCompletedRejectsWrongVideoAssetAndIncompletePrefix(t *testing.T) {
	svc, repo, cmd := historyImportFixture(t)
	cmd.SourceGroupID = "video:video-source"
	if _, err := svc.ImportCompleted(context.Background(), cmd); err == nil || len(repo.jobs) != 0 {
		t.Fatalf("three-image prefix accepted as completed: %v", err)
	}
	cmd.Completed = append(cmd.Completed, HistoricalStep{StepKey: "video", SourceRecordID: "video-source", AssetID: "gen-video"})
	lookup := svc.historyAssets.(historyAssetLookup)
	lookup.assets["gen-video"] = &client.ImportedGeneratedAsset{
		ID: "gen-video", OrganizationID: "other", Scope: "ai_clothing", OwnerUserID: "owner",
		AssetKind: "ai_video", ObjectPath: "materials/other/ai_clothing/imported/gen-video",
		SourceSystem: "media_ai", SourceRecordID: "video-source", SourceSHA256: strings.Repeat("b", 64),
		SourceCreatedAt: time.Now().UnixNano(), SourceProductID: "product", SourceVirtualIPID: "vip",
	}
	if _, err := svc.ImportCompleted(context.Background(), cmd); err == nil || len(repo.jobs) != 0 {
		t.Fatalf("foreign video output accepted: %v", err)
	}
}

func TestImportIncompleteRejectsForeignOrMissingAssetWithoutSaving(t *testing.T) {
	svc, repo, cmd := historyImportFixture(t)
	lookup := svc.historyAssets.(historyAssetLookup)
	lookup.assets[cmd.Completed[1].AssetID].OrganizationID = "another"
	if _, err := svc.ImportIncomplete(context.Background(), cmd); err == nil || len(repo.jobs) != 0 {
		t.Fatalf("foreign asset accepted: err=%v jobs=%d", err, len(repo.jobs))
	}
	lookup.assets[cmd.Completed[1].AssetID].OrganizationID = "org"
	cmd.Completed[1].AssetID = "missing"
	if _, err := svc.ImportIncomplete(context.Background(), cmd); err == nil || len(repo.jobs) != 0 {
		t.Fatalf("missing asset accepted: err=%v jobs=%d", err, len(repo.jobs))
	}
}

func TestImportIncompleteRejectsForeignWorkspace(t *testing.T) {
	svc, repo, cmd := historyImportFixture(t)
	svc.ConfigureHistoryWorkspaces(historyWorkspaceLookup{workspace: &client.WorkspaceContext{ID: "workspace", OrganizationID: "other-org", TeamID: "team"}})
	if _, err := svc.ImportIncomplete(context.Background(), cmd); err == nil || len(repo.jobs) != 0 {
		t.Fatalf("foreign workspace accepted: err=%v jobs=%d", err, len(repo.jobs))
	}
}
