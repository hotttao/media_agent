package template

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/template"
)

type departmentGrants struct {
	domain.GrantRepo
	rows []*domain.Grant
	err  error
}

func (r departmentGrants) ListGrants(context.Context, string) ([]*domain.Grant, error) {
	return r.rows, r.err
}

func TestTemplateDepartmentViewerAndEditorAccess(t *testing.T) {
	for _, permission := range []domain.Permission{domain.PermissionViewer, domain.PermissionEditor} {
		repo := &accessRepo{value: &domain.Template{ID: "tpl", CreatedBy: "owner", Visibility: domain.VisibilityTeam, Status: domain.StatusDraft}}
		svc := NewService(repo, departmentGrants{rows: []*domain.Grant{{TeamID: "dept", Permission: permission}}}, nil, nil, nil)
		if _, err := svc.Get(context.Background(), "tpl", "member", "dept"); err != nil {
			t.Fatal("department read rejected", err)
		}
		if _, err := svc.GetVersionForCaller(context.Background(), "tpl", 1, "member", "dept"); err != nil {
			t.Fatal("department version read rejected", err)
		}
		if _, err := svc.Get(context.Background(), "tpl", "member", "other"); !errors.Is(err, domain.ErrTemplateForbidden) {
			t.Fatal("unrelated department read accepted")
		}
		_, err := svc.Archive(context.Background(), "tpl", "member", "dept")
		if permission == domain.PermissionEditor {
			if err != nil || repo.writes != 1 {
				t.Fatal("editor archive rejected", err)
			}
		} else if !errors.Is(err, domain.ErrTemplateForbidden) || repo.writes != 0 {
			t.Fatal("viewer wrote resource")
		}
		if _, err := svc.Publish(context.Background(), "tpl", "member"); !errors.Is(err, domain.ErrTemplateForbidden) {
			t.Fatal("editor acquired owner publication permission")
		}
	}
}

func TestTemplateGrantLookupFailureDoesNotAuthorizeEditor(t *testing.T) {
	repo := &accessRepo{value: &domain.Template{ID: "tpl", CreatedBy: "owner", Visibility: domain.VisibilityTeam}}
	svc := NewService(repo, departmentGrants{err: errors.New("private infrastructure failure")}, nil, nil, nil)
	if _, err := svc.Archive(context.Background(), "tpl", "member", "dept"); err == nil || err.Error() == "private infrastructure failure" || repo.writes != 0 {
		t.Fatal("grant outage authorized or leaked")
	}
}
