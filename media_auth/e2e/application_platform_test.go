//go:build e2e

package e2e

// Cross-service assembly is test-only: services retain separate databases and HTTP boundaries.
import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"io"
	cfg "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/authn/appcredential"
	gdomain "media_agent/media_agent_gateway/biz/domain/agent_session"
	gclient "media_agent/media_agent_gateway/biz/shared/client"
	gflow "media_agent/media_agent_gateway/biz/workflow"
	apprepo "media_agent/media_auth/biz/dal/repo/application"
	orgrepo "media_agent/media_auth/biz/dal/repo/organization"
	teamrepo "media_agent/media_auth/biz/dal/repo/team"
	internalhandler "media_agent/media_auth/biz/handler/application_internal"
	teamhandler "media_agent/media_auth/biz/handler/team_internal"
	internalrouter "media_agent/media_auth/biz/router/application_internal"
	teamservice "media_agent/media_auth/biz/service/team"
	"media_agent/media_auth/e2e/fixtures"
	cdal "media_agent/media_control/biz/dal"
	crepo "media_agent/media_control/biz/dal/repo/workspace"
	chandler "media_agent/media_control/biz/handler/workspace"
	crouter "media_agent/media_control/biz/router/workspace"
	cservice "media_agent/media_control/biz/service/workspace"
	"media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/secret"
	cflow "media_agent/media_control/biz/workflow/application_agent"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func applicationHTTP(t *testing.T, h *server.Hertz) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, e := io.ReadAll(r.Body)
		if e != nil {
			w.WriteHeader(400)
			return
		}
		headers := []ut.Header{}
		for k, values := range r.Header {
			for _, v := range values {
				headers = append(headers, ut.Header{Key: k, Value: v})
			}
		}
		res := ut.PerformRequest(h.Engine, r.Method, r.URL.RequestURI(), &ut.Body{Body: strings.NewReader(string(body)), Len: len(body)}, headers...).Result()
		res.Header.VisitAll(func(k, v []byte) { w.Header().Set(string(k), string(v)) })
		w.WriteHeader(res.StatusCode())
		_, _ = w.Write(res.Body())
	}))
	t.Cleanup(s.Close)
	return s
}

type applicationSessionFixture struct{ user, workspace string }

func (s applicationSessionFixture) Authorize(_ context.Context, id, user string) (*gdomain.Session, error) {
	if id != "session" || user != s.user {
		return nil, errors.New("not owner")
	}
	return &gdomain.Session{ID: id, UserID: user, WorkspaceID: s.workspace}, nil
}
func TestApplicationAuthControlGatewayHTTPIntegration(t *testing.T) {
	ctx := context.Background()
	f := newApplicationFixture(t)
	teams := teamservice.NewService(teamrepo.NewRepo(f.db), f.tx, orgrepo.NewRepo(f.db))
	f.apps.WithBusiness(apprepo.NewRepo(f.db), applicationTeams{teams})
	secretValue := f.createApp(t, "integration-app")
	cmd := command("integration-app", secretValue, "external")
	p, _, e := f.users.Upsert(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if e := f.db.Exec(sql, args...).Error; e != nil {
			t.Fatal(e)
		}
	}
	exec("INSERT INTO media_auth.organizations(id,name) VALUES ('org','Org')")
	exec("INSERT INTO media_auth.teams(id,name,created_by,organization_id) VALUES ('team','Team',?,'org')", f.admin.ID)
	exec("INSERT INTO media_auth.organization_members(organization_id,user_id,role) VALUES ('org',?,'member')", p.UserID)
	exec("INSERT INTO media_auth.team_members(team_id,user_id,role) VALUES ('team',?,'admin')", p.UserID)
	exec("INSERT INTO media_auth.organization_entitlements(organization_id,feature_key,granted_by) VALUES ('org','workspace.create',?)", f.admin.ID)
	scopes := []string{"workspace.read", "workspace.ensure", "session.chat"}
	if _, e = f.apps.SetGrant(ctx, "admin", "integration-app", "team", scopes, false); e != nil {
		t.Fatal(e)
	}
	token, e := f.users.BusinessToken(ctx, cmd, "team", scopes)
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("MEDIA_AUTH_INTERNAL_TOKEN", "integration-internal")
	oldService, oldSecret, oldTeams, oldEnforce := internalhandler.Service, internalhandler.InternalSecret, teamhandler.TeamSvc, teamhandler.KetoEnforce
	internalhandler.Service = f.apps
	internalhandler.InternalSecret = "integration-internal"
	teamhandler.TeamSvc = teams
	teamhandler.KetoEnforce = false
	t.Cleanup(func() {
		internalhandler.Service = oldService
		internalhandler.InternalSecret = oldSecret
		teamhandler.TeamSvc = oldTeams
		teamhandler.KetoEnforce = oldEnforce
	})
	ah := server.New()
	internalrouter.Register(ah)
	ah.POST("/internal/team-authorization/check", teamhandler.AuthorizeTeam)
	authHTTP := applicationHTTP(t, ah)
	controlDB := fixtures.NewDatabase(t)
	if e = cdal.Migrate("../../media_control/migrations", &cfg.DatabaseConfig{Enabled: true, Driver: "postgres", Dsn: controlDB.DSN, MigrateOnStart: true}); e != nil {
		t.Fatal(e)
	}
	r := crepo.New(controlDB.Gorm)
	tx := cdal.NewTxRunner(controlDB.Gorm)
	workspaces := cservice.New(r, tx, secret.New("test-key"), mediaauth.New(authHTTP.URL, "integration-internal", nil))
	chandler.ConfigureApplications(&cflow.Flow{Workspaces: workspaces, Bindings: cservice.NewApplicationBindings(r), Tx: tx}, appcredential.New(authHTTP.URL, "integration-internal"))
	t.Cleanup(func() { chandler.ConfigureApplications(nil, nil) })
	ch := server.New()
	crouter.Register(ch)
	controlHTTP := applicationHTTP(t, ch)
	request := func(method, path, raw, body string) *http.Response {
		t.Helper()
		req, e := http.NewRequest(method, controlHTTP.URL+path, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+raw)
		req.Header.Set("Content-Type", "application/json")
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		return res
	}
	res := request("PUT", "/apps/v1/agents/assistant", token, `{"display_name":"Assistant","agent_type":"goose"}`)
	var ws struct {
		ID string `json:"id"`
	}
	e = json.NewDecoder(res.Body).Decode(&ws)
	res.Body.Close()
	if res.StatusCode != 201 || e != nil || ws.ID == "" {
		t.Fatalf("create via Auth/Control: %d %v", res.StatusCode, e)
	}
	gateway := &gflow.ApplicationChat{Authority: appcredential.New(authHTTP.URL, "integration-internal"), Workspaces: gclient.ApplicationWorkspace{BaseURL: controlHTTP.URL}, Sessions: applicationSessionFixture{p.UserID, ws.ID}}
	if _, e = gateway.Authorize(ctx, token, "", "session"); e != nil {
		t.Fatal("Gateway application session authorization", e)
	}
	if _, e = gateway.Authorize(ctx, token, "foreign-workspace", ""); e == nil {
		t.Fatal("foreign workspace accepted")
	}
	profile, e := f.users.Token(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	res = request("GET", "/apps/v1/agents/assistant", profile, "")
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("profile token accepted %d", res.StatusCode)
	}
	if _, e = f.apps.SetGrant(ctx, "admin", "integration-app", "team", nil, true); e != nil {
		t.Fatal(e)
	}
	if _, e = gateway.Authorize(ctx, token, "", "session"); e == nil {
		t.Fatal("revoked grant accepted through Gateway")
	}
}
