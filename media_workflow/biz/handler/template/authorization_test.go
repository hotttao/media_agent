package template

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	domain "media_agent/media_workflow/biz/domain/template"
	svc "media_agent/media_workflow/biz/service/template"
	"media_agent/media_workflow/biz/shared/auth"
)

type grantResourceRepo struct {
	domain.Repo
	writes int
}

func (r *grantResourceRepo) Get(context.Context, string) (*domain.Template, error) {
	return &domain.Template{ID: "tpl", CreatedBy: "owner", Visibility: domain.VisibilityPrivate}, nil
}
func (r *grantResourceRepo) UpdateStatus(context.Context, string, domain.Status) error {
	r.writes++
	return nil
}

type grantSink struct {
	writes int
	issuer string
}

func (s *grantSink) AddGrant(_ context.Context, grant *domain.Grant) error {
	s.writes++
	s.issuer = grant.GrantedBy
	return nil
}
func (s *grantSink) RemoveGrant(context.Context, string, string) error           { s.writes++; return nil }
func (s *grantSink) ListGrants(context.Context, string) ([]*domain.Grant, error) { return nil, nil }

func TestTemplateHandlerIgnoresSpoofedGrantOwnerAndAuthenticatesWrites(t *testing.T) {
	repo, sink := &grantResourceRepo{}, &grantSink{}
	previous := Svc
	Svc = svc.NewService(repo, sink, nil, nil, nil)
	t.Cleanup(func() { Svc = previous })
	h := server.Default()
	// Explicit local principal injector for handler wiring only; does not
	// claim to test real Gateway JWT verification or IAM membership.
	h.Use(auth.New(&auth.Config{DevUserID: "attacker"}).Handler())
	h.POST("/api/v1/templates/:id/grants", GrantTemplate)
	h.POST("/api/v1/templates/:id/archive", ArchiveTemplate)
	h.POST("/api/v1/templates/:id/publish", PublishTemplate)
	h.PUT("/api/v1/templates/:id", UpdateTemplate)
	for _, request := range []struct{ method, path, body string }{
		{"POST", "/api/v1/templates/tpl/grants?granted_by=owner", `{"team_id":"team","permission":"editor","granted_by":"owner"}`},
		{"POST", "/api/v1/templates/tpl/archive", `{}`},
		{"POST", "/api/v1/templates/tpl/publish", `{}`},
		{"PUT", "/api/v1/templates/tpl", `{"name":"changed","prompt_body":"changed","platform":"jimeng","gen_way":"i2i"}`},
	} {
		response := ut.PerformRequest(h.Engine, request.method, request.path, &ut.Body{Body: strings.NewReader(request.body), Len: len(request.body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
		if response.Result().StatusCode() != 403 || repo.writes != 0 || sink.writes != 0 {
			t.Fatal("spoofed or unauthorized write accepted")
		}
	}
	owner := server.Default()
	owner.Use(auth.New(&auth.Config{DevUserID: "owner"}).Handler())
	owner.POST("/api/v1/templates/:id/archive", ArchiveTemplate)
	owner.POST("/api/v1/templates/:id/grants", GrantTemplate)
	response := ut.PerformRequest(owner.Engine, "POST", "/api/v1/templates/tpl/archive", &ut.Body{Body: strings.NewReader(`{}`), Len: 2}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if response.Result().StatusCode() != 200 || repo.writes != 1 {
		t.Fatal("authenticated owner identity was not passed to service")
	}
	body := `{"team_id":"team","permission":"viewer","granted_by":"attacker"}`
	response = ut.PerformRequest(owner.Engine, "POST", "/api/v1/templates/tpl/grants?granted_by=attacker", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
	if response.Result().StatusCode() != 200 || sink.writes != 1 || sink.issuer != "owner" {
		t.Fatal("grant issuer came from client payload")
	}
}
