package agent_product_integration

import (
	"context"
	"errors"
	"media_agent/hertz_infra/metadataartifact"
	"media_agent/media_control/biz/dal"
	metadataRepo "media_agent/media_control/biz/dal/repo/agent_configuration"
	workspaceRepo "media_agent/media_control/biz/dal/repo/workspace"
	cfgdomain "media_agent/media_control/biz/domain/agent_configuration"
	metadataService "media_agent/media_control/biz/service/agent_configuration"
	workspaceService "media_agent/media_control/biz/service/workspace"
	authClient "media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/secret"
	metadataFlow "media_agent/media_control/biz/workflow/metadata_update"
	creation "media_agent/media_control/biz/workflow/workspace_creation"
	"testing"
)

type rejectedInitialConfiguration struct{}

func (rejectedInitialConfiguration) ConfirmInTransaction(context.Context, cfgdomain.Snapshot, string, string, string, int64) (*cfgdomain.Release, error) {
	return nil, errors.New("injected initial configuration failure")
}
func TestWorkspaceCreationInitialReleaseAndAtomicFailure(t *testing.T) {
	db, _ := database(t)
	auth := identityFacts(t)
	defer auth.Close()
	workspaces := workspaceService.New(workspaceRepo.New(db), dal.NewTxRunner(db), secret.New("product-key"), authClient.New(auth.URL, "auth-internal", nil))
	metadata := metadataService.New(metadataRepo.New(db), dal.NewTxRunner(db))
	flow := &creation.Flow{Workspaces: workspaces, Metadata: metadata, Tx: dal.NewTxRunner(db)}
	cmd := workspaceService.CreateCommand{UserID: "owner", OrganizationID: "org", TeamID: "workspace-team", Name: "Ready by default", AgentType: "codex"}
	value, err := flow.Create(context.Background(), cmd)
	must(t, err)
	if value.AgentConfiguration.Version != 2 {
		t.Fatal("initial configuration version not returned")
	}
	artifacts, err := metadataartifact.New(t.TempDir())
	must(t, err)
	selection, version, err := metadataFlow.New(metadata, workspaces, artifacts, nil).Selection(context.Background(), value.Workspace.ID, "owner")
	must(t, err)
	if version != 2 || selection.Prompt == nil || selection.Prompt.Mode != "none" || len(selection.Resources) != 0 || selection.PlatformPromptTemplateVersion != "2" {
		t.Fatal("workspace lacks safe initial Release")
	}
	_, _, target, err := workspaces.RuntimeContextVersion(context.Background(), value.Workspace.ID, "owner", 0)
	must(t, err)
	if target != selection.TargetContextVersion || target == "" {
		t.Fatal("new session cannot pin initial metadata")
	}
	flow.Metadata = rejectedInitialConfiguration{}
	if _, err = flow.Create(context.Background(), cmd); err == nil {
		t.Fatal("initial Release failure accepted")
	}
	for _, table := range []string{"workspaces", "agent_configurations", "agent_metadata_releases", "metadata_notification_outbox"} {
		var count int64
		must(t, db.Table(table).Count(&count).Error)
		if count != 1 {
			t.Fatalf("partial creation in %s: %d rows", table, count)
		}
	}
}
