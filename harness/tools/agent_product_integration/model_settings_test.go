package agent_product_integration

import (
	"context"
	"media_agent/media_control/biz/dal"
	ar "media_agent/media_control/biz/dal/repo/agent_configuration"
	wr "media_agent/media_control/biz/dal/repo/workspace"
	as "media_agent/media_control/biz/service/agent_configuration"
	ws "media_agent/media_control/biz/service/workspace"
	auth "media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/secret"
	flow "media_agent/media_control/biz/workflow/workspace_creation"
	"testing"
)

func TestModelInvocationSettingsHistoryAndAtomicRelease(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	identity := identityFacts(t)
	defer identity.Close()
	workspaces := ws.New(wr.New(db), dal.NewTxRunner(db), secret.New("model-settings-test"), auth.New(identity.URL, "auth-internal", nil))
	metadata := as.New(ar.New(db), dal.NewTxRunner(db))
	f := &flow.Flow{Workspaces: workspaces, Metadata: metadata, Tx: dal.NewTxRunner(db)}
	v, e := f.Create(ctx, ws.CreateCommand{UserID: "owner", OrganizationID: "org", TeamID: "workspace-team", Name: "Goose", AgentType: "goose", APIKey: "private-model-key", ModelProtocol: "openai", ModelName: "model-one", ModelBaseURL: "https://api.example.test/v1"})
	must(t, e)
	oldVersion := v.AgentConfiguration.Version
	name := "model-two"
	next, e := f.Update(ctx, ws.UpdateCommand{WorkspaceID: v.Workspace.ID, UserID: "owner", Name: "Goose", AgentType: "goose", ModelName: &name})
	must(t, e)
	if next.AgentConfiguration.Version <= oldVersion {
		t.Fatal("model change did not advance fixed configuration")
	}
	old, oldKey, _, e := workspaces.RuntimeContextVersion(ctx, v.Workspace.ID, "owner", oldVersion)
	must(t, e)
	latest, key, target, e := workspaces.RuntimeContextVersion(ctx, v.Workspace.ID, "owner", 0)
	must(t, e)
	if old.ModelName != "model-one" || latest.ModelName != "model-two" || latest.ModelProtocol != "openai" || latest.ModelBaseURL != "https://api.example.test/v1" || key != "private-model-key" || oldKey != key || target == "" {
		t.Fatal("fixed model settings or credentials were not preserved")
	}
	release, _, e := metadata.Get(ctx, v.Workspace.ID, v.AgentConfiguration.ID, "", 0, false)
	must(t, e)
	if release.SelectionRevision != latest.AgentConfiguration.Version || release.Prompt == nil || release.Prompt.Mode != "none" {
		t.Fatal("model update lost the atomic metadata release")
	}
	if e = db.Table("agent_configurations").Where("id=?", v.AgentConfiguration.ID).Update("model_name", "unversioned-change").Error; e == nil {
		t.Fatal("unversioned model mutation accepted")
	}
	invalid := "https://user:private-secret@api.example.test/v1"
	if _, e = f.Update(ctx, ws.UpdateCommand{WorkspaceID: v.Workspace.ID, UserID: "owner", Name: "Goose", AgentType: "goose", ModelBaseURL: &invalid}); e == nil {
		t.Fatal("credential-bearing endpoint accepted")
	}
	fixed, _, _, e := workspaces.RuntimeContextVersion(ctx, v.Workspace.ID, "owner", 0)
	must(t, e)
	if fixed.ModelName != "model-two" || fixed.AgentConfiguration.Version != latest.AgentConfiguration.Version {
		t.Fatal("invalid change partially committed")
	}
}
