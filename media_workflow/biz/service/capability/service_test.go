package capability

import (
	"context"
	"errors"
	domain "media_agent/media_workflow/biz/domain/capability"
	"testing"
)

type fakeRepo struct {
	rows []*domain.Capability
	err  error
}

func (r fakeRepo) ListPublished(context.Context) ([]*domain.Capability, error) { return r.rows, r.err }
func (r fakeRepo) Get(_ context.Context, key string, version int) (*domain.Capability, error) {
	if r.err != nil {
		return nil, r.err
	}
	for _, row := range r.rows {
		if row.Key == key && row.Version == version {
			return row, nil
		}
	}
	return nil, domain.ErrCapabilityNotFound
}
func fixture(version int) *domain.Capability {
	return &domain.Capability{Key: "jimeng.i2i.multi_image_prompt", Version: version, Provider: "jimeng", Mode: "image_generation", GenerationMethod: domain.ImageToImage, GenerationType: domain.MultiImagePrompt, ExecutionTarget: domain.ExecutionTargetPersonalWorker, OutputMediaType: domain.MediaImage, ConductorTaskDef: "private-task", Status: "published", Inputs: []domain.Input{{Name: "prompt", Type: domain.InputString, Required: true}, {Name: "images", Type: domain.InputResourceRefList, Required: true, MediaType: domain.MediaImage, MinItems: 1}}}
}
func TestCatalogRequiresIdentityAndValidQueries(t *testing.T) {
	s := NewService(fakeRepo{})
	if _, err := s.List(context.Background(), "", ListQuery{}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("anonymous catalog accepted")
	}
	for _, q := range []ListQuery{{Page: -1}, {PageSize: 201}, {ExecutionTarget: "internal"}, {GenerationMethod: "model_image"}, {GenerationType: "render"}} {
		if _, err := s.List(context.Background(), "user", q); !errors.Is(err, ErrInvalidQuery) {
			t.Fatal("invalid query accepted", q)
		}
	}
	if _, err := s.GetVersion(context.Background(), "user", "key", 0); !errors.Is(err, ErrInvalidQuery) {
		t.Fatal("nonpositive version accepted")
	}
}
func TestCatalogPublishedProviderIdentityAndStablePagination(t *testing.T) {
	draft := fixture(3)
	draft.Status = "draft"
	legacy := &domain.Capability{Key: "media.image.model", Version: 1, Status: "published", OutputAssetKind: "model_image"}
	rows := []*domain.Capability{fixture(2), draft, legacy, fixture(1)}
	s := NewService(fakeRepo{rows: rows})
	page, err := s.List(context.Background(), "user", ListQuery{Provider: "jimeng", GenerationMethod: "i2i", GenerationType: "multi_image_prompt", ExecutionTarget: "personal_worker", Page: 2, PageSize: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].Version != 2 {
		t.Fatal("filter/pagination identity lost", page, err)
	}
	empty, err := s.List(context.Background(), "user", ListQuery{Page: 3, PageSize: 1})
	if err != nil || len(empty.Items) != 0 || empty.Total != 2 {
		t.Fatal("empty page lost total")
	}
	for _, item := range []*domain.Capability{draft, legacy} {
		if _, err := s.GetVersion(context.Background(), "user", item.Key, item.Version); !errors.Is(err, domain.ErrCapabilityNotFound) {
			t.Fatal("nonpublic contract exposed")
		}
	}
	found, err := s.GetVersion(context.Background(), "user", fixture(1).Key, 1)
	if err != nil || found.Ref() != "jimeng.i2i.multi_image_prompt@1" {
		t.Fatal("exact version not returned")
	}
}
func TestCatalogAuthorityFailureDoesNotBecomeEmptySuccess(t *testing.T) {
	fault := errors.New("repo fault")
	s := NewService(fakeRepo{err: fault})
	if _, err := s.List(context.Background(), "user", ListQuery{}); !errors.Is(err, fault) {
		t.Fatal("failure hidden")
	}
}
