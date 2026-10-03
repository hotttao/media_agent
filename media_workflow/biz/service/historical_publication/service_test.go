package historical_publication

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	publication "media_agent/media_workflow/biz/domain/historical_publication"
	jobdomain "media_agent/media_workflow/biz/domain/job"
	"media_agent/media_workflow/biz/shared/client"
)

type fakeRepo struct {
	item *publication.HistoricalVideoPush
}

func (r *fakeRepo) GetBySource(_ context.Context, workspaceID, sourceSystem, sourceRecordID string) (*publication.HistoricalVideoPush, error) {
	if r.item != nil && r.item.WorkspaceID == workspaceID && r.item.SourceSystem == sourceSystem && r.item.SourceRecordID == sourceRecordID {
		return r.item, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (r *fakeRepo) Get(_ context.Context, id string) (*publication.HistoricalVideoPush, error) {
	if r.item != nil && r.item.ID == id {
		return r.item, nil
	}
	return nil, gorm.ErrRecordNotFound
}
func (r *fakeRepo) List(context.Context, string, int, int) ([]*publication.HistoricalVideoPush, int64, error) {
	return []*publication.HistoricalVideoPush{r.item}, 1, nil
}
func (r *fakeRepo) Save(_ context.Context, item *publication.HistoricalVideoPush) error {
	r.item = item
	return nil
}

type fakeJobs struct{ items map[string]*jobdomain.Job }

func (j fakeJobs) Get(_ context.Context, id string) (*jobdomain.Job, error) {
	if job := j.items[id]; job != nil {
		return job, nil
	}
	return nil, gorm.ErrRecordNotFound
}

type fakeWorkspaces struct{ allowed bool }

func (w fakeWorkspaces) GetWorkspaceForUser(_ context.Context, workspaceID, userID string) (*client.WorkspaceContext, error) {
	if w.allowed && workspaceID == "workspace" && userID == "owner" {
		return &client.WorkspaceContext{ID: "workspace", OrganizationID: "org"}, nil
	}
	return nil, gorm.ErrRecordNotFound
}

type fakeObjects struct{ calls int }

func (o *fakeObjects) ResolveInternalObject(_ context.Context, org, path string) (client.ResolvedStorageObject, error) {
	o.calls++
	if org != "org" || !strings.HasPrefix(path, "materials/org/ai_clothing/historical_publication_import/") {
		return client.ResolvedStorageObject{}, errors.New("foreign object")
	}
	return client.ResolvedStorageObject{URL: "http://storage.example/object", ExpiresAt: time.Now().Add(time.Minute).Unix()}, nil
}

func fixture() (*Service, *fakeRepo, *fakeObjects, ImportCommand) {
	repo, objects := &fakeRepo{}, &fakeObjects{}
	jobs := fakeJobs{items: map[string]*jobdomain.Job{
		"job-1": {ID: "job-1", Origin: "legacy_import", OrganizationID: "org", WorkspaceID: "workspace", SubmittedBy: "owner", SourceSystem: "media_ai", SourceRecordID: "video:source-1", Status: jobdomain.JobSucceeded,
			Input: jobdomain.JobInput{Product: &jobdomain.ProductData{ID: "product"}, VirtualIP: &jobdomain.VirtualIpData{ID: "vip"}}},
	}}
	cmd := ImportCommand{OrganizationID: "org", WorkspaceID: "workspace", OwnerUserID: "owner", SourceRecordID: "push-1",
		ProductID: "product", VirtualIPID: "vip", Title: "发布文案", SourceCreatedAt: time.Now().Add(-time.Hour).UTC(), SourceUpdatedAt: time.Now().UTC(),
		Video:   publication.MediaObject{ObjectPath: "materials/org/ai_clothing/historical_publication_import/clip.mp4", SHA256: strings.Repeat("a", 64)},
		Members: []publication.Member{{Ordinal: 0, SourceVideoID: "source-1", JobID: "job-1"}},
	}
	return New(repo, jobs, fakeWorkspaces{allowed: true}, objects), repo, objects, cmd
}

func TestImportHistoricalVideoPushIsIdempotentAndChecksMembers(t *testing.T) {
	svc, repo, objects, cmd := fixture()
	first, err := svc.Import(context.Background(), cmd)
	if err != nil || first == nil || len(first.Members) != 1 || first.SourceDigest == "" || objects.calls != 1 {
		t.Fatalf("valid historical push rejected: push=%+v err=%v", first, err)
	}
	second, err := svc.Import(context.Background(), cmd)
	if err != nil || second.ID != first.ID || objects.calls != 1 {
		t.Fatalf("same source did not reuse record: push=%+v err=%v", second, err)
	}
	cmd.Title = "changed"
	if _, err := svc.Import(context.Background(), cmd); !errors.Is(err, ErrConflict) || repo.item.ID != first.ID {
		t.Fatalf("changed source overwrote historical push: %v", err)
	}
	svc, repo, _, cmd = fixture()
	cmd.Members[0].SourceVideoID = "wrong-source"
	if _, err := svc.Import(context.Background(), cmd); err == nil || repo.item != nil {
		t.Fatalf("wrong Video Job source accepted: %v", err)
	}
	svc, repo, _, cmd = fixture()
	cmd.Video.ObjectPath = "materials/another/ai_clothing/historical_publication_import/clip.mp4"
	if _, err := svc.Import(context.Background(), cmd); err == nil || repo.item != nil {
		t.Fatalf("foreign publication media accepted: %v", err)
	}
}

func TestHistoricalVideoPushReadRequiresWorkspace(t *testing.T) {
	svc, _, _, cmd := fixture()
	item, err := svc.Import(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(context.Background(), item.ID, "intruder"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign user read historical push: %v", err)
	}
	if _, _, err := svc.List(context.Background(), "workspace", "intruder", 1, 20); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign user listed historical pushes: %v", err)
	}
}
