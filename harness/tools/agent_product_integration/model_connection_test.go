package agent_product_integration

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/media_control/biz/dal"
	ar "media_agent/media_control/biz/dal/repo/agent_configuration"
	mr "media_agent/media_control/biz/dal/repo/model_connection"
	wr "media_agent/media_control/biz/dal/repo/workspace"
	md "media_agent/media_control/biz/domain/model_connection"
	as "media_agent/media_control/biz/service/agent_configuration"
	ms "media_agent/media_control/biz/service/model_connection"
	ws "media_agent/media_control/biz/service/workspace"
	auth "media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/secret"
	flow "media_agent/media_control/biz/workflow/workspace_creation"
	"strings"
	"testing"
)

func TestPersonalModelConnectionEncryptionBindingHistoryAndRevocation(t *testing.T) {
	db, _ := database(t)
	ctx := context.Background()
	identity := identityFacts(t)
	defer identity.Close()
	cipher := secret.New("model-connection-test")
	connections := ms.New(mr.New(db), cipher)
	raw := `{"tokens":{"access_token":"private-access","refresh_token":"private-refresh"},"auth_mode":"chatgpt"}`
	v, e := connections.Create(ctx, "owner", "Codex", raw)
	must(t, e)
	body, _ := json.Marshal(v)
	if strings.Contains(string(body), "private-access") {
		t.Fatal("credential exposed")
	}
	var encrypted []byte
	must(t, db.Raw("SELECT encrypted_auth FROM model_connections WHERE id=?", v.ID).Row().Scan(&encrypted))
	if strings.Contains(string(encrypted), "private-access") {
		t.Fatal("plaintext persisted")
	}
	other, e := connections.List(ctx, "other")
	must(t, e)
	if len(other) != 0 {
		t.Fatal("other owner can list connection")
	}
	if _, e = connections.Get(ctx, v.ID, "other"); !errors.Is(e, md.ErrNotFound) {
		t.Fatal("other owner can read connection")
	}
	if e = connections.ValidateBinding(ctx, v.ID, "other", "codex"); !errors.Is(e, md.ErrNotFound) {
		t.Fatal("other owner can bind connection")
	}
	if e = connections.ValidateBinding(ctx, v.ID, "owner", "claude-code"); e == nil {
		t.Fatal("wrong runtime accepted")
	}
	for _, bad := range []string{`{}`, `[]`, `{"tokens":{"access_token":"one"}}`, strings.Repeat("x", 65537)} {
		if _, e = connections.Create(ctx, "owner", "bad", bad); e == nil {
			t.Fatal("invalid authentication accepted")
		}
	}
	second, e := connections.Create(ctx, "other", "Other Codex", raw)
	must(t, e)
	must(t, db.Table("model_connections").Where("id=?", second.ID).Update("encrypted_auth", encrypted).Error)
	if _, e = connections.Resolve(ctx, second.ID); !errors.Is(e, md.ErrInvalid) {
		t.Fatal("ciphertext scope substitution accepted")
	}
	workspaces := ws.New(wr.New(db), dal.NewTxRunner(db), cipher, auth.New(identity.URL, "auth-internal", nil)).WithModelConnections(connections)
	metadata := as.New(ar.New(db), dal.NewTxRunner(db))
	create := &flow.Flow{Workspaces: workspaces, Metadata: metadata, Tx: dal.NewTxRunner(db)}
	workspace, e := create.Create(ctx, ws.CreateCommand{UserID: "owner", OrganizationID: "org", TeamID: "workspace-team", Name: "Codex", AgentType: "codex"})
	must(t, e)
	updated, e := create.Update(ctx, ws.UpdateCommand{WorkspaceID: workspace.Workspace.ID, UserID: "owner", Name: "Codex", AgentType: "codex", ModelConnectionID: &v.ID})
	must(t, e)
	fixed, _, target, e := workspaces.RuntimeContextVersion(ctx, workspace.Workspace.ID, "owner", 0)
	must(t, e)
	if fixed.ModelConnectionID != v.ID || target == "" || fixed.AgentConfiguration.Version != updated.AgentConfiguration.Version {
		t.Fatal("selected connection is not executable")
	}
	previous, _, _, e := workspaces.RuntimeContextVersion(ctx, workspace.Workspace.ID, "owner", 2)
	must(t, e)
	if previous.ModelConnectionID != "" {
		t.Fatal("old configuration changed")
	}
	current, _, e := metadata.Get(ctx, workspace.Workspace.ID, workspace.AgentConfiguration.ID, "", 0, false)
	must(t, e)
	if current.Prompt == nil || current.Prompt.Mode != "none" || len(current.Resources) != 0 {
		t.Fatal("binding changed metadata content")
	}
	replacement := `{"tokens":{"access_token":"new-access","refresh_token":"new-refresh"}}`
	v, e = connections.Update(ctx, v.ID, "owner", "Renamed", replacement, 1, false)
	must(t, e)
	resolved, e := connections.Resolve(ctx, v.ID)
	must(t, e)
	if resolved.Version != 2 || resolved.AuthJSON != replacement {
		t.Fatal("replacement not loaded")
	}
	if _, e = connections.Update(ctx, v.ID, "owner", "Lost update", replacement, 1, false); !errors.Is(e, md.ErrConflict) {
		t.Fatal("stale update accepted")
	}
	v, e = connections.Update(ctx, v.ID, "owner", "Name only", "", 2, false)
	must(t, e)
	resolved, e = connections.Resolve(ctx, v.ID)
	must(t, e)
	if resolved.Version != 2 || v.Version != 3 {
		t.Fatal("rename reset native credential revision")
	}
	v, e = connections.Update(ctx, v.ID, "owner", "", "", 3, true)
	must(t, e)
	if _, e = connections.Resolve(ctx, v.ID); !errors.Is(e, md.ErrRevoked) {
		t.Fatal("revoked authentication resolved")
	}
	if len(v.EncryptedAuth) != 0 {
		t.Fatal("revoked secret retained")
	}
}
