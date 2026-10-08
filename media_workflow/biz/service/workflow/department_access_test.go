package workflow

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/workflow"
)

type departmentResourceRepo struct {
	domain.Repo
	resource *domain.Workflow
	writes   int
	query    domain.ListQuery
}

func (r *departmentResourceRepo) Get(context.Context, string) (*domain.Workflow, error) {
	return r.resource, nil
}
func (r *departmentResourceRepo) UpdateStatus(_ context.Context, _ string, status domain.Status) error {
	r.resource.Status = status
	r.writes++
	return nil
}
func (r *departmentResourceRepo) List(_ context.Context, q domain.ListQuery) ([]*domain.WorkflowSummary, int, error) {
	r.query = q
	return nil, 0, nil
}

type departmentGrantRepo struct {
	domain.GrantRepo
	permission domain.Permission
}

func (r departmentGrantRepo) ListGrants(context.Context, string) ([]*domain.Grant, error) {
	return []*domain.Grant{{TeamID: "dept", Permission: r.permission}}, nil
}

func TestWorkflowDepartmentReadAndEditorDoNotBecomeOwner(t *testing.T) {
	for _, permission := range []domain.Permission{domain.PermissionViewer, domain.PermissionEditor} {
		repo := &departmentResourceRepo{resource: &domain.Workflow{ID: "wf", CreatedBy: "owner", Visibility: domain.VisibilityTeam}}
		svc := NewService(repo, departmentGrantRepo{permission: permission}, nil, nil, nil)
		if _, err := svc.Get(context.Background(), "wf", "member", "dept"); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Get(context.Background(), "wf", "member", "other"); !errors.Is(err, domain.ErrWorkflowForbidden) {
			t.Fatal("wrong department read authorized")
		}
		_, err := svc.Archive(context.Background(), "wf", "member", "dept")
		if permission == domain.PermissionEditor {
			if err != nil || repo.writes != 1 {
				t.Fatal("editor archive rejected")
			}
		} else if !errors.Is(err, domain.ErrWorkflowForbidden) || repo.writes != 0 {
			t.Fatal("viewer archive accepted")
		}
		if _, err := svc.Publish(context.Background(), "wf", "member"); !errors.Is(err, domain.ErrWorkflowForbidden) {
			t.Fatal("department editor became publication owner")
		}
	}
}

func TestWorkflowListAccessFactsOverrideQueryValues(t *testing.T) {
	repo := &departmentResourceRepo{}
	svc := NewService(repo, nil, nil, nil, nil)
	_, _, err := svc.List(context.Background(), domain.ListQuery{CallerID: "forged-owner", TeamIDs: []string{"forged-team"}}, "member", "dept")
	if err != nil || !repo.query.EnforceAccess || repo.query.CallerID != "member" || len(repo.query.TeamIDs) != 1 || repo.query.TeamIDs[0] != "dept" {
		t.Fatal("untrusted query identity reached repository")
	}
}
