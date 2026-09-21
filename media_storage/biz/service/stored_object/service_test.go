package stored_object

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	domain "media_agent/media_storage/biz/domain/stored_object"
)

type fakeDriver struct{ putPath string }

func (d *fakeDriver) Put(_ context.Context, path string, body io.Reader) (domain.BlobInfo, error) {
	d.putPath = path
	data, _ := io.ReadAll(body)
	return domain.BlobInfo{ObjectPath: path, SizeBytes: int64(len(data)), Checksum: "sha256:test"}, nil
}
func (*fakeDriver) Open(context.Context, string) (io.ReadCloser, error) { return nil, nil }
func (*fakeDriver) Stat(context.Context, string) (domain.BlobInfo, error) {
	return domain.BlobInfo{}, nil
}
func (*fakeDriver) Delete(context.Context, string) error { return nil }

type fakeRepo struct{ saved domain.StoredObject }

func (r *fakeRepo) SaveActive(_ context.Context, object domain.StoredObject) (domain.StoredObject, error) {
	r.saved = object
	return object, nil
}

func TestPutPersistsMetadataAfterDriverCommit(t *testing.T) {
	driver, repo := &fakeDriver{}, &fakeRepo{}
	service := NewService(driver, repo, "local_file")
	got, err := service.Put(context.Background(), "materials/org/ai_clothing/material/key.png", "image/png", strings.NewReader("png"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if driver.putPath != got.ObjectPath || repo.saved.Namespace != "materials" || repo.saved.ContentType != "image/png" || repo.saved.Driver != "local_file" {
		t.Fatalf("unexpected stored object: %+v", repo.saved)
	}
}

func TestPutRejectsOtherNamespace(t *testing.T) {
	service := NewService(&fakeDriver{}, &fakeRepo{}, "local_file")
	_, err := service.Put(context.Background(), "workflow/org/file.png", "image/png", strings.NewReader("png"))
	if !errors.Is(err, domain.ErrNamespaceForbidden) {
		t.Fatalf("error=%v, want ErrNamespaceForbidden", err)
	}
}
