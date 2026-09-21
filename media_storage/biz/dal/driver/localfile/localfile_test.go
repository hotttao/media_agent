// LocalFile driver behavior tests.
package localfile

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "media_agent/media_storage/biz/domain/stored_object"
)

func TestDriverPutOpenStatDelete(t *testing.T) {
	driver, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	info, err := driver.Put(ctx, "materials/org_1/image.png", strings.NewReader("image bytes"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.SizeBytes != int64(len("image bytes")) || !strings.HasPrefix(info.Checksum, "sha256:") {
		t.Fatalf("unexpected info: %+v", info)
	}
	reader, err := driver.Open(ctx, info.ObjectPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(got) != "image bytes" {
		t.Fatalf("read = %q, %v", got, err)
	}
	if err := driver.Delete(ctx, info.ObjectPath); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := driver.Delete(ctx, info.ObjectPath); err != nil {
		t.Fatalf("idempotent Delete: %v", err)
	}
	if _, err := driver.Stat(ctx, info.ObjectPath); !errors.Is(err, domain.ErrObjectNotFound) {
		t.Fatalf("Stat error = %v, want ErrObjectNotFound", err)
	}
}

func TestDriverPutIsIdempotentButRejectsDifferentContent(t *testing.T) {
	driver, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	path := "materials/org_1/image.png"
	first, err := driver.Put(ctx, path, strings.NewReader("same"))
	if err != nil {
		t.Fatalf("first Put: %v", err)
	}
	second, err := driver.Put(ctx, path, strings.NewReader("same"))
	if err != nil || second.Checksum != first.Checksum {
		t.Fatalf("idempotent Put = %+v, %v", second, err)
	}
	if _, err := driver.Put(ctx, path, strings.NewReader("different")); !errors.Is(err, domain.ErrObjectConflict) {
		t.Fatalf("different Put error = %v, want ErrObjectConflict", err)
	}
}

func TestDriverRejectsUnsafePaths(t *testing.T) {
	driver, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, value := range []string{"", "single", "/absolute/file", "materials/../outside", "materials\\outside"} {
		if _, err := driver.Put(context.Background(), value, strings.NewReader("x")); !errors.Is(err, domain.ErrInvalidObjectPath) {
			t.Errorf("Put(%q) error = %v, want ErrInvalidObjectPath", value, err)
		}
	}
}

func TestDriverRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "materials")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	driver, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = driver.Put(context.Background(), "materials/org_1/image.png", strings.NewReader("x"))
	if !errors.Is(err, domain.ErrInvalidObjectPath) {
		t.Fatalf("Put error = %v, want ErrInvalidObjectPath", err)
	}
}

func TestDriverHonorsCanceledContext(t *testing.T) {
	driver, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = driver.Put(ctx, "materials/org_1/image.png", strings.NewReader("x"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Put error = %v, want context.Canceled", err)
	}
}
