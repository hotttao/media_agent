//go:build e2e

package e2e

import (
	"context"
	"errors"
	apprepo "media_agent/media_auth/biz/dal/repo/application"
	orgrepo "media_agent/media_auth/biz/dal/repo/organization"
	teamrepo "media_agent/media_auth/biz/dal/repo/team"
	d "media_agent/media_auth/biz/domain/application"
	teamservice "media_agent/media_auth/biz/service/team"
	"testing"
)

type applicationTeams struct{ service *teamservice.Service }

func (a applicationTeams) Allowed(ctx context.Context, user, team, permission string) (string, bool, error) {
	v, e := a.service.Authorize(ctx, user, team, permission)
	return v.OrganizationID, v.Allowed && v.TeamRole != "" && v.OrganizationRole != "", e
}
func TestApplicationBusinessGrantCurrentPermissions(t *testing.T) {
	f := newApplicationFixture(t)
	ctx := context.Background()
	f.apps.WithBusiness(apprepo.NewRepo(f.db), applicationTeams{teamservice.NewService(teamrepo.NewRepo(f.db), f.tx, orgrepo.NewRepo(f.db))})
	secret := f.createApp(t, "business-app")
	cmd := command("business-app", secret, "customer-1")
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
	exec("INSERT INTO media_auth.organizations(id,name) VALUES ('business-org','Business')")
	exec("INSERT INTO media_auth.teams(id,name,created_by,organization_id) VALUES ('business-team','Team',?,'business-org')", f.admin.ID)
	scopes := []string{"workspace.read", "workspace.ensure", "session.chat"}
	if _, e = f.apps.SetGrant(ctx, "user", "business-app", "business-team", scopes, false); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("non-admin grant: %v", e)
	}
	if _, e = f.apps.SetGrant(ctx, "admin", "business-app", "business-team", scopes, false); e != nil {
		t.Fatal(e)
	}
	issue := func(scopes []string) (string, error) { return f.users.BusinessToken(ctx, cmd, "business-team", scopes) }
	if _, e = issue(scopes); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("grant must not create membership: %v", e)
	}
	exec("INSERT INTO media_auth.organization_members(organization_id,user_id,role) VALUES ('business-org',?,'member')", p.UserID)
	exec("INSERT INTO media_auth.team_members(team_id,user_id,role) VALUES ('business-team',?,'admin')", p.UserID)
	if _, e = issue(scopes); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("missing entitlement: %v", e)
	}
	exec("INSERT INTO media_auth.organization_entitlements(organization_id,feature_key,granted_by) VALUES ('business-org','workspace.create',?)", f.admin.ID)
	raw, e := issue(scopes)
	if e != nil {
		t.Fatal(e)
	}
	verified, e := f.apps.VerifyBusiness(ctx, raw, "session.chat")
	if e != nil || verified.UserID != p.UserID || verified.TeamID != "business-team" {
		t.Fatalf("verify business: %+v %v", verified, e)
	}
	profile, e := f.users.Token(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.VerifyBusiness(ctx, profile, "workspace.read"); e == nil {
		t.Fatal("profile token gained business access")
	}
	if e = f.apps.SetMCPTarget(ctx, "admin", "business-app", "business-mcp", "https://business.example", false); e != nil {
		t.Fatal(e)
	}
	caller, enabled, e := f.apps.ResolveCaller(ctx, "business-mcp", "https://business.example/mcp", p.UserID, "business-org", "business-team")
	if e != nil || !enabled || caller.ExternalID != "customer-1" {
		t.Fatalf("Caller mapping: %+v %v %v", caller, enabled, e)
	}
	if _, _, e = f.apps.ResolveCaller(ctx, "business-mcp", "https://attacker.example/mcp", p.UserID, "business-org", "business-team"); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("Caller wrong origin: %v", e)
	}
	if _, enabled, e = f.apps.ResolveCaller(ctx, "unregistered", "https://business.example/mcp", p.UserID, "business-org", "business-team"); e != nil || enabled {
		t.Fatal("unregistered target acquired Caller")
	}
	otherSecret := f.createApp(t, "business-other")
	other, _, e := f.users.Upsert(ctx, command("business-other", otherSecret, "customer-1"))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.apps.ResolveCaller(ctx, "business-mcp", "https://business.example/mcp", other.UserID, "business-org", "business-team"); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("cross-app Caller: %v", e)
	}
	if e = f.apps.SetMCPTarget(ctx, "admin", "business-other", "business-mcp", "https://attacker.example", false); !errors.Is(e, d.ErrConflict) {
		t.Fatalf("target takeover: %v", e)
	}
	exec("UPDATE media_auth.team_members SET role='member' WHERE user_id=?", p.UserID)
	if _, _, e = f.apps.ResolveCaller(ctx, "business-mcp", "https://business.example/mcp", p.UserID, "business-org", "business-team"); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("Caller role revoked: %v", e)
	}
	if _, e = f.apps.VerifyBusiness(ctx, raw, "session.chat"); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("role removal: %v", e)
	}
	if _, e = f.apps.VerifyBusiness(ctx, raw, "workspace.read"); e != nil {
		t.Fatal(e)
	}
	exec("DELETE FROM media_auth.organization_entitlements WHERE organization_id='business-org'")
	if _, e = f.apps.VerifyBusiness(ctx, raw, "workspace.ensure"); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("entitlement removal: %v", e)
	}
	if _, e = f.apps.SetGrant(ctx, "admin", "business-app", "business-team", []string{"workspace.read"}, false); e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.VerifyBusiness(ctx, raw, "workspace.read"); e == nil {
		t.Fatal("grant mutation did not revoke old credential")
	}
	read, e := issue([]string{"workspace.read"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.apps.VerifyBusiness(ctx, read, "session.chat"); !errors.Is(e, d.ErrForbidden) {
		t.Fatalf("scope escalation: %v", e)
	}
	exec("DELETE FROM media_auth.organization_members WHERE user_id=?", p.UserID)
	if _, e = f.apps.VerifyBusiness(ctx, read, "workspace.read"); e == nil {
		t.Fatal("removed organization membership accepted")
	}
}
