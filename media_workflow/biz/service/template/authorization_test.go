package template

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_workflow/biz/domain/template"
)

type accessRepo struct {
	domain.Repo
	value                *domain.Template
	writes, versionReads int
}

func (r *accessRepo) Get(context.Context, string) (*domain.Template, error) { return r.value, nil }
func (r *accessRepo) UpdateStatus(_ context.Context, _ string, status domain.Status) error {
	r.writes++
	r.value.Status = status
	return nil
}
func (r *accessRepo) Delete(context.Context, string) error { r.writes++; return nil }
func (r *accessRepo) GetVersion(context.Context, string, int) (*domain.TemplateVersion, error) {
	r.versionReads++
	return &domain.TemplateVersion{TemplateID: r.value.ID, Version: 1, PromptBody: "private snapshot"}, nil
}

func TestTemplateNonOwnerCannotWriteOrReadPrivateVersions(t *testing.T) {
	for _, caller := range []string{"other", ""} {
		for _, operation := range []string{"update", "archive", "publish", "delete", "version"} {
			t.Run(caller+"/"+operation, func(t *testing.T) {
				repo := &accessRepo{value: &domain.Template{ID: "tpl", CreatedBy: "owner", Visibility: domain.VisibilityPrivate, Status: domain.StatusDraft}}
				svc := NewService(repo, nil, nil, nil, nil)
				var err error
				switch operation {
				case "update":
					_, err = svc.Update(context.Background(), UpdateCommand{ID: "tpl", CallerID: caller})
				case "archive":
					_, err = svc.Archive(context.Background(), "tpl", caller)
				case "publish":
					_, err = svc.Publish(context.Background(), "tpl", caller)
				case "delete":
					_, err = svc.Delete(context.Background(), "tpl", caller)
				case "version":
					_, err = svc.GetVersionForCaller(context.Background(), "tpl", 1, caller)
				}
				if !errors.Is(err, domain.ErrTemplateForbidden) || repo.writes != 0 || repo.versionReads != 0 {
					t.Fatal("unauthorized operation reached resource side effects", err)
				}
			})
		}
	}
}

func TestTemplateAuthorizedOwnerAndPublicVersionReadsRemainAvailable(t *testing.T) {
	repo := &accessRepo{value: &domain.Template{ID: "tpl", CreatedBy: "owner", Visibility: domain.VisibilityPrivate}}
	svc := NewService(repo, nil, nil, nil, nil)
	if _, err := svc.GetVersionForCaller(context.Background(), "tpl", 1, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Archive(context.Background(), "tpl", "owner"); err != nil || repo.writes != 1 {
		t.Fatal("owner archive denied", err)
	}
	repo.value.Visibility = domain.VisibilityPublic
	if _, err := svc.GetVersionForCaller(context.Background(), "tpl", 1, "viewer"); err != nil || repo.versionReads != 2 {
		t.Fatal("public viewer denied", err)
	}
	if _, err := svc.GetVersionForCaller(context.Background(), "tpl", 1, ""); !errors.Is(err, domain.ErrTemplateForbidden) {
		t.Fatal("missing principal accepted")
	}
}
