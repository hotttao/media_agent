//go:build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	configpb "media_agent/hertz_gen/config"
	"media_agent/media_auth/biz/dal"
	apprepo "media_agent/media_auth/biz/dal/repo/application"
	authrepo "media_agent/media_auth/biz/dal/repo/auth"
	identityrepo "media_agent/media_auth/biz/dal/repo/identity"
	d "media_agent/media_auth/biz/domain/application"
	auth "media_agent/media_auth/biz/domain/auth"
	identity "media_agent/media_auth/biz/domain/identity"
	handler "media_agent/media_auth/biz/handler/application"
	"media_agent/media_auth/biz/middleware"
	router "media_agent/media_auth/biz/router/application"
	appservice "media_agent/media_auth/biz/service/application"
	authservice "media_agent/media_auth/biz/service/auth"
	identityservice "media_agent/media_auth/biz/service/identity"
	tokenservice "media_agent/media_auth/biz/service/token"
	"media_agent/media_auth/biz/shared/session"
	"media_agent/media_auth/biz/workflow"
	"media_agent/media_auth/e2e/fixtures"
	"os"
	"strings"
	"sync"
	"testing"
)

type applicationFixture struct {
	db         *gorm.DB
	apps       *appservice.Service
	users      *workflow.ApplicationUsers
	identities *identityservice.Service
	userRepo   auth.Repository
	tx         dal.TxRunner
	admin      auth.User
}

func newApplicationFixture(t *testing.T) *applicationFixture {
	t.Helper()
	db := fixtures.NewDatabase(t)
	fixtures.Migrate(t, db)
	db.Gorm = db.Gorm.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	repo := authrepo.NewRepo(db.Gorm)
	admin, e := repo.CreateUser(context.Background(), "admin", "Admin", "", "", "admin")
	if e != nil {
		t.Fatal("create fixture admin")
	}
	tx := dal.NewTxRunner(db.Gorm)
	apps := appservice.NewService(apprepo.NewRepo(db.Gorm), tx)
	ids := identityservice.NewService(identityrepo.NewRepo(db.Gorm))
	return &applicationFixture{db.Gorm, apps, workflow.NewApplicationUsers(apps, authservice.NewManagedUsers(authrepo.NewManagedRepo(db.Gorm)), ids, tx), ids, repo, tx, admin}
}
func (f *applicationFixture) createApp(t *testing.T, id string) string {
	t.Helper()
	_, secret, e := f.apps.Create(context.Background(), f.admin.ID, "admin", id, id)
	if e != nil {
		t.Fatal("create app failed")
	}
	return secret
}
func command(id, secret, external string) workflow.ApplicationUserCommand {
	return workflow.ApplicationUserCommand{ClientID: id, Secret: secret, ExternalID: external, DisplayName: "Alice", Email: "same@example.test"}
}
func TestApplicationIsolationLifecycleAndConcurrency(t *testing.T) {
	f := newApplicationFixture(t)
	ctx := context.Background()
	a := f.createApp(t, "app-a")
	b := f.createApp(t, "app-b")
	cmd := command("app-a", a, "17")
	const n = 12
	out := make(chan string, n)
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); p, _, e := f.users.Upsert(ctx, cmd); out <- p.UserID; errs <- e }()
	}
	wg.Wait()
	close(out)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("concurrent upsert failed: %T", e)
		}
	}
	var id string
	for v := range out {
		if id != "" && v != id {
			t.Fatal("concurrent upsert split ownership")
		}
		id = v
	}
	var count int64
	f.db.Table("media_auth.users").Where("managed_by = ?", "app-a").Count(&count)
	if count != 1 {
		t.Fatal("orphan or duplicate managed User")
	}
	p, _, e := f.users.Upsert(ctx, command("app-b", b, "17"))
	if e != nil || p.UserID == id {
		t.Fatal("cross-application collision")
	}
	bad := cmd
	bad.ClientID = "app-b"
	if _, e = f.users.Get(ctx, bad); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("secret crossed application boundary")
	}
	if _, e = f.users.Get(ctx, command("app-b", b, "not-owned")); !errors.Is(e, d.ErrNotFound) {
		t.Fatal("foreign mapping visible")
	}
	if e = f.identities.DeleteIdentity(ctx, id, "app:app-a"); !errors.Is(e, identity.ErrProtected) {
		t.Fatal("application identity was unbound")
	}
	if e = f.identities.BindIdentity(ctx, id, "app:app-a", "other", "", nil); !errors.Is(e, identity.ErrProtected) {
		t.Fatal("application identity was overwritten")
	}
	if e = f.userRepo.SetPassword(ctx, id, "hash"); !errors.Is(e, auth.ErrManagedUser) {
		t.Fatal("managed user gained password")
	}
	if _, e = f.userRepo.Upsert(ctx, "app-user:"+id, "LDAP User", ""); !errors.Is(e, auth.ErrManagedUser) {
		t.Fatal("LDAP claimed a managed user")
	}
	kratos, e := f.userRepo.ResolveKratosIdentity(ctx, "kratos-alice", "same@example.test", "Alice", false)
	if e != nil || kratos.ID == id || kratos.ID == p.UserID || kratos.ManagedBy != "" {
		t.Fatal("Kratos merged by managed email")
	}
	if e = f.db.Exec("UPDATE media_auth.users SET kratos_identity_id='forged' WHERE id=?", id).Error; e == nil {
		t.Fatal("database allowed interactive identity")
	}
	token, e := f.users.Token(ctx, cmd)
	if e != nil {
		t.Fatal("issue failed")
	}
	if profile, e := f.apps.Profile(ctx, token); e != nil || profile.UserID != id {
		t.Fatal("profile failed")
	}
	for _, raw := range []string{a, "not-a-token", token + "extra"} {
		if _, e = f.apps.Profile(ctx, raw); !errors.Is(e, d.ErrUnauthorized) {
			t.Fatal("wrong credential type accepted")
		}
	}
	if _, e = f.apps.SetStatus(ctx, "admin", "app-a", "disabled"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.Profile(ctx, token); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("disabled app credential accepted")
	}
	if _, e = f.users.Token(ctx, cmd); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("disabled app issued credential")
	}
	if _, e = f.apps.SetStatus(ctx, "admin", "app-a", "active"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.Profile(ctx, token); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("app recovery revived old credential")
	}
	token, e = f.users.Token(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	_, rotated, e := f.apps.Rotate(ctx, "admin", "app-a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.users.Token(ctx, cmd); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("old app secret accepted")
	}
	if _, e = f.apps.Profile(ctx, token); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("rotation failed to revoke")
	}
	cmd.Secret = rotated
	token, e = f.users.Token(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.userRepo.UpdateStatus(ctx, id, "active", "disabled"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.Profile(ctx, token); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("disabled user accepted")
	}
	if _, _, e = f.users.Upsert(ctx, cmd); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("upsert revived user")
	}
	if _, e = f.userRepo.UpdateStatus(ctx, id, "disabled", "active"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.Profile(ctx, token); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("user recovery revived credential")
	}
	token, e = f.users.Token(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.db.Exec("UPDATE media_auth.application_access_credentials SET expires_at=now()-interval '1 second' WHERE application_id='app-a'").Error; e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.Profile(ctx, token); !errors.Is(e, d.ErrUnauthorized) {
		t.Fatal("expired credential accepted")
	}
	if _, e = f.userRepo.SoftDeleteUser(ctx, id, f.admin.ID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.users.Upsert(ctx, cmd); !errors.Is(e, d.ErrForbidden) {
		t.Fatal("deleted mapping was recreated")
	}
}

type failingAppIdentity struct {
	ApplicationIdentities workflow.ApplicationIdentities
}

func (f failingAppIdentity) FindUserIDentity(ctx context.Context, p, e string) (string, error) {
	return f.ApplicationIdentities.FindUserIDentity(ctx, p, e)
}
func (f failingAppIdentity) CreateApplicationIdentity(context.Context, string, string, string) error {
	return errors.New("injected identity persistence failure")
}
func TestApplicationProvisioningRollback(t *testing.T) {
	f := newApplicationFixture(t)
	secret := f.createApp(t, "rollback")
	broken := workflow.NewApplicationUsers(f.apps, authservice.NewManagedUsers(authrepo.NewManagedRepo(f.db)), failingAppIdentity{f.identities}, f.tx)
	if _, _, e := broken.Upsert(context.Background(), command("rollback", secret, "17")); e == nil {
		t.Fatal("expected identity failure")
	}
	var count int64
	f.db.Table("media_auth.users").Where("managed_by='rollback'").Count(&count)
	if count != 0 {
		t.Fatal("transaction leaked an orphan User")
	}
	if _, created, e := f.users.Upsert(context.Background(), command("rollback", secret, "17")); e != nil || !created {
		t.Fatal("retry did not recover")
	}

	down, err := os.ReadFile("../migrations/0018_applications.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(string(down)), "BEGIN;"), "COMMIT;")
	err = f.db.Transaction(func(tx *gorm.DB) error { return tx.Exec(body).Error })
	if err == nil {
		t.Fatal("down migration removed managed identity boundary")
	}
	if _, err = f.users.Get(context.Background(), command("rollback", secret, "17")); err != nil {
		t.Fatal("failed downgrade damaged identity facts")
	}
}
func TestApplicationRegisteredHTTPBoundary(t *testing.T) {
	f := newApplicationFixture(t)
	signer, e := session.NewSigner(&configpb.AuthConfig{JwtSecret: "application-http-test-secret"})
	if e != nil {
		t.Fatal(e)
	}
	cookie, e := signer.Sign(auth.Session{Sub: f.admin.ID, Role: "admin", AuthVersion: f.admin.AuthVersion})
	if e != nil {
		t.Fatal(e)
	}
	oldS, oldU := handler.Service, handler.Users
	handler.Service = f.apps
	handler.Users = f.users
	t.Cleanup(func() { handler.Service = oldS; handler.Users = oldU })
	h := server.New()
	h.Use(middleware.ApplicationProtocol(middleware.SessionWithToken(signer, tokenservice.NewService(f.userRepo))))
	router.Register(h)
	request := func(method, path, body, authorization, cookieValue string, want int) map[string]any {
		t.Helper()
		headers := []ut.Header{{Key: "Content-Type", Value: "application/json"}}
		if authorization != "" {
			headers = append(headers, ut.Header{Key: "Authorization", Value: authorization})
		}
		if cookieValue != "" {
			headers = append(headers, ut.Header{Key: "Cookie", Value: "token=" + cookieValue})
		}
		var b *ut.Body
		if body != "" {
			b = &ut.Body{Body: strings.NewReader(body), Len: len(body)}
		}
		resp := ut.PerformRequest(h.Engine, method, path, b, headers...).Result()
		if resp.StatusCode() != want {
			t.Fatalf("%s %s got %d want %d", method, path, resp.StatusCode(), want)
		}
		if strings.HasPrefix(path, "/apps/") && string(resp.Header.Peek("Cache-Control")) != "no-store" {
			t.Fatal("credential response is cacheable")
		}
		result := map[string]any{}
		if len(resp.Body()) > 0 {
			if e := json.Unmarshal(resp.Body(), &result); e != nil {
				t.Fatal("invalid JSON response")
			}
		}
		return result
	}
	request("POST", "/api/applications", `{"id":"http-app","name":"HTTP"}`, "", "", 401)
	userCookie, _ := signer.Sign(auth.Session{Sub: f.admin.ID, Role: "user", AuthVersion: f.admin.AuthVersion})
	request("POST", "/api/applications", `{"id":"http-app","name":"HTTP"}`, "", userCookie, 403)
	created := request("POST", "/api/applications", `{"id":"http-app","name":"HTTP"}`, "", cookie, 201)
	secret := created["client_secret"].(string)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("http-app:"+secret))
	request("POST", "/api/applications", `{"id":"http-app","name":"HTTP"}`, "", cookie, 409)
	listing := request("GET", "/api/applications", "", "", cookie, 200)
	encoded, _ := json.Marshal(listing)
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "secret_hash") {
		t.Fatal("listing exposed credential")
	}
	request("PUT", "/apps/v1/users/17", `{"display_name":"Alice"}`, "", cookie, 401)
	request("PUT", "/apps/v1/users/17", `{"display_name":"Alice"}`, "Basic invalid", cookie, 401)
	user := request("PUT", "/apps/v1/users/17", `{"display_name":"Alice","external_id":"evil","application_id":"other","user_id":"admin","role":"admin"}`, basic, "", 201)
	if user["external_id"] != "17" || user["application_id"] != "http-app" {
		t.Fatal("body overrode authenticated identity")
	}
	request("GET", "/apps/v1/users/evil", "", basic, "", 404)
	response := request("POST", "/apps/v1/users/17/token", "", basic, "", 200)
	token := response["access_token"].(string)
	if response["expires_in"] != float64(3600) || response["scope"] != "profile" {
		t.Fatal("unexpected credential scope")
	}
	request("GET", "/apps/v1/me", "", basic, cookie, 401)
	request("GET", "/apps/v1/me", "", "Bearer "+cookie, cookie, 401)
	profile := request("GET", "/apps/v1/me", "", "Bearer "+token, "", 200)
	if profile["id"] != user["id"] {
		t.Fatal("wrong represented user")
	}
	request("GET", "/api/applications", "", "Bearer "+token, cookie, 401)
	managedCookie, _ := signer.Sign(auth.Session{Sub: user["id"].(string), Role: "admin", AuthVersion: 1})
	request("GET", "/api/applications", "", "", managedCookie, 401)
	request("PATCH", "/api/applications/http-app/status", `{"status":"disabled"}`, "", cookie, 200)
	request("GET", "/apps/v1/me", "", "Bearer "+token, cookie, 401)
	request("POST", "/apps/v1/users/17/token", "", basic, cookie, 401)
	// Faulted authoritative dependency fails closed and returns no infrastructure error.
	sqlDB, _ := f.db.DB()
	if e := sqlDB.Close(); e != nil {
		t.Fatal(e)
	}
	fault := request("GET", "/apps/v1/me", "", "Bearer "+token, cookie, 503)
	if fmt.Sprint(fault["error"]) != "application service unavailable" {
		t.Fatal("internal database error leaked")
	}
}
