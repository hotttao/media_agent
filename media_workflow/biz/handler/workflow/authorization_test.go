package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	domain "media_agent/media_workflow/biz/domain/workflow"
	svc "media_agent/media_workflow/biz/service/workflow"
	"media_agent/media_workflow/biz/shared/auth"
)

type grantRepoFixture struct{ domain.Repo }

func (grantRepoFixture) Get(context.Context, string) (*domain.Workflow, error) {
	return &domain.Workflow{ID: "wf", CreatedBy: "owner"}, nil
}

type grantWriterFixture struct {
	writes int
	issuer string
}

func (s *grantWriterFixture) AddGrant(_ context.Context, grant *domain.Grant) error {
	s.writes++
	s.issuer = grant.GrantedBy
	return nil
}
func (s *grantWriterFixture) RemoveGrant(context.Context, string, string) error {
	s.writes++
	return nil
}
func (s *grantWriterFixture) ListGrants(context.Context, string) ([]*domain.Grant, error) {
	return nil, nil
}

func TestWorkflowGrantOwnerComesFromAuthenticatedContext(t *testing.T) {
	sink := &grantWriterFixture{}
	previous := Svc
	Svc = svc.NewService(grantRepoFixture{}, sink, nil, nil, nil)
	t.Cleanup(func() { Svc = previous })
	for _, caller := range []string{"attacker", "owner"} {
		h := server.Default()
		h.Use(auth.New(&auth.Config{DevUserID: caller}).Handler())
		h.POST("/api/v1/workflows/:id/grants", GrantWorkflow)
		body := `{"team_id":"team","permission":"viewer","granted_by":"owner"}`
		response := ut.PerformRequest(h.Engine, "POST", "/api/v1/workflows/wf/grants?granted_by=owner", &ut.Body{Body: strings.NewReader(body), Len: len(body)}, ut.Header{Key: "Content-Type", Value: "application/json"})
		if caller == "attacker" {
			if response.Result().StatusCode() != 403 || sink.writes != 0 {
				t.Fatal("forged owner granted workflow")
			}
		} else if response.Result().StatusCode() != 200 || sink.writes != 1 || sink.issuer != "owner" {
			t.Fatal("real owner grant rejected")
		}
	}
}
