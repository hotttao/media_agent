package team_internal

import (
	"context"
	"encoding/json"
	domain "media_agent/media_auth/biz/domain/organization"
	service "media_agent/media_auth/biz/service/organization"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestAuthorizeInternalFailsClosed(t *testing.T) {
	t.Setenv("MEDIA_AUTH_INTERNAL_TOKEN", "internal-secret")

	withoutToken := app.NewContext(0)
	if authorizeInternal(withoutToken) {
		t.Fatal("missing internal token must be rejected")
	}
	if got := withoutToken.Response.StatusCode(); got != 401 {
		t.Fatalf("missing token status=%d, want 401", got)
	}

	wrongToken := app.NewContext(0)
	wrongToken.Request.Header.Set("X-Media-Auth-Internal-Token", "wrong")
	if authorizeInternal(wrongToken) {
		t.Fatal("wrong internal token must be rejected")
	}

	validToken := app.NewContext(0)
	validToken.Request.Header.Set("X-Media-Auth-Internal-Token", "internal-secret")
	if !authorizeInternal(validToken) {
		t.Fatal("matching internal token must be accepted")
	}
}

func TestAuthorizeInternalRejectsUnsetConfiguration(t *testing.T) {
	t.Setenv("MEDIA_AUTH_INTERNAL_TOKEN", "")
	c := app.NewContext(0)
	if authorizeInternal(c) {
		t.Fatal("unset internal token configuration must fail closed")
	}
}

type organizationFactsRepo struct{ domain.Repository }

func (organizationFactsRepo) ListForUser(_ context.Context, user string) ([]domain.Summary, error) {
	if user != "user" {
		return nil, nil
	}
	return []domain.Summary{
		{Organization: domain.Organization{ID: "org", Status: "active"}, MyRole: "admin"},
		{Organization: domain.Organization{ID: "disabled", Status: "disabled"}, MyRole: "admin"},
	}, nil
}
func TestListUserOrganizationsReturnsOnlyCurrentActiveRoles(t *testing.T) {
	t.Setenv("MEDIA_AUTH_INTERNAL_TOKEN", "internal-secret")
	previous := OrganizationSvc
	OrganizationSvc = service.NewService(organizationFactsRepo{})
	t.Cleanup(func() { OrganizationSvc = previous })
	c := app.NewContext(0)
	c.Request.Header.Set("X-Media-Auth-Internal-Token", "internal-secret")
	c.Request.SetBodyString(`{"user_id":"user"}`)
	ListUserOrganizations(context.Background(), c)
	var response struct {
		IDs           []string `json:"organization_ids"`
		Organizations []struct {
			ID   string `json:"organization_id"`
			Role string `json:"role"`
		} `json:"organizations"`
	}
	if err := json.Unmarshal(c.Response.Body(), &response); err != nil {
		t.Fatal(err)
	}
	if c.Response.StatusCode() != 200 || len(response.IDs) != 1 || response.IDs[0] != "org" || len(response.Organizations) != 1 || response.Organizations[0].Role != "admin" {
		t.Fatalf("unexpected identity facts: %s", c.Response.Body())
	}
}
