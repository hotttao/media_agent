package agent_product_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"gorm.io/gorm"
	"io"
	cfg "media_agent/hertz_gen/config"
	metadataPB "media_agent/hertz_gen/model/media_control/agent_metadata"
	promptPB "media_agent/hertz_gen/model/media_control/prompt"
	"media_agent/hertz_infra/authn/resourceaccess"
	"media_agent/hertz_infra/metadataartifact"
	"media_agent/media_control/biz/dal"
	metadataRepo "media_agent/media_control/biz/dal/repo/agent_configuration"
	promptRepo "media_agent/media_control/biz/dal/repo/prompt"
	workspaceRepo "media_agent/media_control/biz/dal/repo/workspace"
	metadataHandler "media_agent/media_control/biz/handler/agent_metadata"
	promptHandler "media_agent/media_control/biz/handler/prompt"
	"media_agent/media_control/biz/middleware"
	metadataRouter "media_agent/media_control/biz/router/agent_metadata"
	promptRouter "media_agent/media_control/biz/router/prompt"
	metadataService "media_agent/media_control/biz/service/agent_configuration"
	promptService "media_agent/media_control/biz/service/prompt"
	workspaceService "media_agent/media_control/biz/service/workspace"
	authClient "media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/secret"
	metadataFlow "media_agent/media_control/biz/workflow/metadata_update"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func database(t *testing.T) (*gorm.DB, *cfg.DatabaseConfig) {
	t.Helper()
	dsn := os.Getenv("MEDIA_AGENT_PRODUCT_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	admin, err := dal.NewDB(&cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: dsn})
	must(t, err)
	adminSQL, err := admin.DB()
	must(t, err)
	t.Cleanup(func() { _ = adminSQL.Close() })
	name := fmt.Sprintf("agent_product_%d", time.Now().UnixNano())
	must(t, admin.Exec("CREATE DATABASE "+name).Error)
	parsed, err := url.Parse(dsn)
	must(t, err)
	parsed.Path = "/" + name
	config := &cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: parsed.String(), MigrateOnStart: true}
	db, err := dal.NewDB(config)
	must(t, err)
	t.Cleanup(func() {
		sql, err := db.DB()
		if err == nil {
			_ = sql.Close()
		}
		_ = admin.Exec("DROP DATABASE " + name + " WITH (FORCE)").Error
	})
	must(t, dal.Migrate("../../../media_control/migrations", config))
	return db, config
}
func mount(h *server.Hertz) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := []ut.Header{}
		for k, vs := range r.Header {
			for _, v := range vs {
				headers = append(headers, ut.Header{Key: k, Value: v})
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "request read failed", 400)
			return
		}
		rec := ut.PerformRequest(h.Engine, r.Method, r.URL.RequestURI(), &ut.Body{Body: bytes.NewReader(body), Len: len(body)}, headers...)
		rec.Header().VisitAll(func(k, v []byte) { w.Header().Add(string(k), string(v)) })
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	}))
}
func call(t *testing.T, base, user, method, path string, body any, status int, out any) {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		must(t, err)
	}
	req, err := http.NewRequest(method, base+path, bytes.NewReader(encoded))
	must(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Control-Service-Token", "product-internal")
	req.Header.Set("X-Control-Acting-User-ID", user)
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	must(t, err)
	if resp.StatusCode != status {
		t.Fatalf("%s %s status=%d expected=%d body=%s", method, path, resp.StatusCode, status, content)
	}
	if out != nil {
		reflect.ValueOf(out).Elem().SetZero()
		must(t, json.Unmarshal(content, out))
	}
}
func identityFacts(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Media-Auth-Internal-Token") != "auth-internal" {
			http.Error(w, "forbidden", 401)
			return
		}
		var in map[string]string
		must(t, json.NewDecoder(r.Body).Decode(&in))
		user := in["user_id"]
		switch r.URL.Path {
		case "/internal/organizations:byUser":
			ids := []string{"org"}
			if user == "outsider" {
				ids = []string{"foreign-org"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"organization_ids": ids})
		case "/internal/teams:byUser":
			_ = json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{user + "-team"}})
		case "/internal/team-authorization/check":
			team := in["team_id"]
			org := "org"
			if team == "foreign-team" {
				org = "foreign-org"
			}
			role := ""
			if team == user+"-team" {
				role = "member"
			}
			allowed := role != "" || team == "workspace-team" && user != "outsider"
			if in["permission"] == "manage" {
				allowed = team == "workspace-team" && (user == "owner" || user == "editor" || user == "peer")
			}
			_ = json.NewEncoder(w).Encode(resourceaccess.Team{TeamID: team, OrganizationID: org, TeamRole: role, OrganizationActive: true, TeamActive: true, Allowed: allowed})
		default:
			http.NotFound(w, r)
		}
	}))
}
func TestPromptLibraryAndExplicitConfiguration(t *testing.T) {
	db, _ := database(t)
	auth := identityFacts(t)
	defer auth.Close()
	prompts := promptService.New(promptRepo.New(db), dal.NewTxRunner(db), resourceaccess.New(auth.URL, "auth-internal", nil))
	workspace := workspaceService.New(workspaceRepo.New(db), dal.NewTxRunner(db), secret.New("product-key"), authClient.New(auth.URL, "auth-internal", nil))
	artifacts, err := metadataartifact.New(t.TempDir())
	must(t, err)
	flow := metadataFlow.New(metadataService.New(metadataRepo.New(db), dal.NewTxRunner(db)), workspace, artifacts, nil).WithPrompts(prompts)
	promptHandler.Configure(prompts)
	metadataHandler.Configure(flow)
	middleware.Configure("", "product-internal")
	h := server.New()
	promptRouter.Register(h)
	metadataRouter.Register(h)
	httpServer := mount(h)
	defer httpServer.Close()
	base := httpServer.URL
	must(t, db.Exec("INSERT INTO workspaces(id,owner_user_id,created_by,organization_id,team_id,name,desired_state,version,created_at,updated_at) VALUES('workspace','owner','owner','org','workspace-team','Workspace','running',1,now(),now())").Error)
	must(t, db.Exec("INSERT INTO agent_configurations(id,workspace_id,agent_id,agent_type,version,created_at,updated_at) VALUES('config','workspace','codex','codex',1,now(),now())").Error)
	var created promptPB.PromptResponse
	call(t, base, "owner", "POST", "/api/v1/prompts", map[string]any{"organization_id": "org", "name": "Writer", "content": "old", "visibility": "team", "grants": []map[string]string{{"team_id": "editor-team", "permission": "editor"}, {"team_id": "viewer-team", "permission": "viewer"}}}, 201, &created)
	if created.OrganizationId != "org" || created.MyPermission != "owner" || created.Version != 1 {
		t.Fatal(created.String())
	}
	path := "/api/v1/prompts/" + created.Id
	var visible promptPB.ListPromptsResponse
	call(t, base, "editor", "GET", "/api/v1/prompts?organization_id=org", nil, 200, &visible)
	if len(visible.Prompts) != 1 || visible.Prompts[0].MyPermission != "editor" {
		t.Fatal(visible.String())
	}
	call(t, base, "outsider", "GET", path, nil, 404, nil)
	call(t, base, "viewer", "PUT", path, map[string]any{"name": "bad", "content": "bad", "expected_version": 1}, 403, nil)
	var updated promptPB.PromptResponse
	call(t, base, "editor", "PUT", path, map[string]any{"name": "Writer", "content": "new", "expected_version": 1}, 200, &updated)
	call(t, base, "editor", "PUT", path+"/access", map[string]any{"visibility": "public", "expected_version": 2}, 403, nil)
	call(t, base, "owner", "PUT", path+"/access", map[string]any{"visibility": "team", "grants": []map[string]string{{"team_id": "foreign-team", "permission": "viewer"}}, "expected_version": 2}, 403, nil)
	var selection metadataPB.MetadataContext
	confirm := func(user, key string, version int64, prompt map[string]any, status int, out any) {
		call(t, base, user, "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", map[string]any{"agent_config_id": "config", "expected_version": version, "idempotency_key": key, "prompt": prompt}, status, out)
	}
	// An editor of the Workspace can select a resource shared by a different owner.
	confirm("editor", "library", 1, map[string]any{"mode": "library", "resource_id": created.Id, "resource_version": 1}, 200, &selection)
	if selection.ConfiguredPrompt != "old" || selection.SelectionRevision != 2 {
		t.Fatal(selection.String())
	}
	var current metadataPB.MetadataSelection
	call(t, base, "viewer", "GET", "/api/v1/workspaces/workspace/agent-metadata", nil, 200, &current)
	if current.Prompt.Mode != "library" || current.Prompt.ResourceVersion != 1 {
		t.Fatal(current.String())
	}
	call(t, base, "peer", "GET", "/api/v1/workspaces/workspace/agent-metadata", nil, 403, nil)
	confirm("peer", "unreadable", 2, map[string]any{"mode": "library", "resource_id": created.Id, "resource_version": 1}, 403, nil)
	confirm("viewer", "no-manage", 2, map[string]any{"mode": "custom", "content": "not allowed"}, 403, nil)
	confirm("editor", "custom", 2, map[string]any{"mode": "custom", "content": "  custom\n"}, 200, &selection)
	if selection.ConfiguredPrompt != "  custom\n" {
		t.Fatal(selection.String())
	}
	var repeated metadataPB.MetadataContext
	confirm("editor", "custom", 2, map[string]any{"mode": "custom", "content": "  custom\n"}, 200, &repeated)
	if repeated.TargetContextVersion != selection.TargetContextVersion {
		t.Fatal("retry duplicated release")
	}
	confirm("editor", "custom", 2, map[string]any{"mode": "custom", "content": "changed"}, 409, nil)
	confirm("editor", "clear", 3, map[string]any{"mode": "none"}, 200, &selection)
	if selection.ConfiguredPrompt != "" {
		t.Fatal("explicit clear retained old content")
	}
	confirm("editor", "conflicting", 4, map[string]any{"mode": "none", "content": "invalid"}, 422, nil)
	// Release snapshots remain independent of later library rollback.
	call(t, base, "editor", "POST", path+"/rollback", map[string]any{"target_version": 1, "expected_version": 2}, 200, &updated)
	if updated.Version != 3 || updated.Content != "old" {
		t.Fatal(updated.String())
	}
	var history promptPB.ListPromptVersionsResponse
	call(t, base, "viewer", "GET", path+"/versions", nil, 200, &history)
	if len(history.Versions) != 3 || history.Versions[0].Version != 3 || history.Versions[1].Content != "new" {
		t.Fatal(history.String())
	}
	var original promptPB.PromptVersion
	call(t, base, "viewer", "GET", path+"/versions/1", nil, 200, &original)
	if original.Content != "old" {
		t.Fatal(original.String())
	}
	call(t, base, "owner", "PUT", path, map[string]any{"name": "stale", "content": "stale", "expected_version": 2}, 409, nil)
	var outboxCount int64
	must(t, db.Table("metadata_notification_outbox").Count(&outboxCount).Error)
	if outboxCount != 3 {
		t.Fatalf("outbox count=%d", outboxCount)
	}
	call(t, base, "owner", "PUT", path+"/access", map[string]any{"visibility": "private", "expected_version": 3}, 200, &updated)
	call(t, base, "viewer", "GET", path+"/versions", nil, 404, nil)
	call(t, base, "owner", "POST", path+"/archive", map[string]any{"expected_version": 4}, 200, &updated)
	call(t, base, "owner", "PUT", path, map[string]any{"name": "x", "content": "x", "expected_version": 5}, 409, nil)
	// Context keys only carry immutable prompt data, never provider credentials.
	release, _, err := flow.Selection(context.Background(), "workspace", "owner")
	must(t, err)
	if release.ConfiguredPrompt != "" || release.Prompt.Mode != "none" {
		t.Fatal("latest selection is not clear")
	}
	// Unscoped legacy rows remain owner-only until their owner assigns a real organization.
	must(t, db.Exec("INSERT INTO prompts(id,owner_user_id,name,content,status,version,created_at,updated_at) VALUES('legacy','owner','Legacy','preserved','active',1,now(),now())").Error)
	var unassigned promptPB.ListPromptsResponse
	call(t, base, "owner", "GET", "/api/v1/prompts/unassigned", nil, 200, &unassigned)
	if len(unassigned.Prompts) != 1 || unassigned.Prompts[0].Id != "legacy" {
		t.Fatal(unassigned.String())
	}
	call(t, base, "editor", "GET", "/api/v1/prompts/legacy", nil, 404, nil)
	call(t, base, "editor", "POST", "/api/v1/prompts/legacy/assign-organization", map[string]any{"organization_id": "org", "expected_version": 1}, 404, nil)
	call(t, base, "owner", "POST", "/api/v1/prompts/legacy/assign-organization", map[string]any{"organization_id": "foreign-org", "expected_version": 1}, 404, nil)
	call(t, base, "owner", "POST", "/api/v1/prompts/legacy/assign-organization", map[string]any{"organization_id": "org", "expected_version": 1}, 200, &updated)
	if updated.OrganizationId != "org" || updated.Content != "preserved" || updated.Version != 2 {
		t.Fatal(updated.String())
	}
	call(t, base, "owner", "POST", "/api/v1/prompts/legacy/assign-organization", map[string]any{"organization_id": "org", "expected_version": 2}, 409, nil)
}
