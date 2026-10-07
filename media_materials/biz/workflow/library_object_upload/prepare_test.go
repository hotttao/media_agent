package library_object_upload

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"media_agent/media_materials/biz/shared/objectstorage"
)

type prepareStorage struct {
	*fakeStorage
	input objectstorage.UploadInput
	calls int
}

func (s *prepareStorage) PresignPut(_ context.Context, in objectstorage.UploadInput) (objectstorage.UploadGrant, error) {
	s.input, s.calls = in, s.calls+1
	return objectstorage.UploadGrant{ObjectPath: in.ObjectPath}, nil
}

func validPrepare() PrepareCommand {
	return PrepareCommand{OrganizationID: "org", Scope: "ai_clothing", UploadKey: strings.Repeat("a", 64), Checksum: strings.Repeat("b", 64), ContentMD5: base64.StdEncoding.EncodeToString(make([]byte, 16)), ContentType: "image/png", SizeBytes: 256}
}

func TestPrepareUsesTaskAndContentBoundNamespace(t *testing.T) {
	storage := &prepareStorage{fakeStorage: &fakeStorage{}}
	w := New(storage)
	c := validPrepare()
	first, err := w.Prepare(context.Background(), c)
	if err != nil || !strings.HasPrefix(first.ObjectPath, "materials/org/ai_clothing/worker_generated/"+c.UploadKey+"/") {
		t.Fatal("upload namespace not derived from trusted task and enterprise", err)
	}
	again, err := w.Prepare(context.Background(), c)
	if err != nil || first.ObjectPath != again.ObjectPath {
		t.Fatal("same manifest lost stable object identity")
	}
	c.Checksum = strings.Repeat("c", 64)
	next, err := w.Prepare(context.Background(), c)
	if err != nil || first.ObjectPath == next.ObjectPath {
		t.Fatal("different content could overwrite prior reference")
	}
}

func TestPrepareRejectsMalformedOrOversizedManifestBeforeStorage(t *testing.T) {
	for _, change := range []func(*PrepareCommand){
		func(c *PrepareCommand) { c.OrganizationID = "../other" }, func(c *PrepareCommand) { c.Scope = "a/b" },
		func(c *PrepareCommand) { c.UploadKey = "arbitrary-path" }, func(c *PrepareCommand) { c.Checksum = "invalid" },
		func(c *PrepareCommand) { c.ContentMD5 = "invalid" }, func(c *PrepareCommand) { c.SizeBytes = 0 },
		func(c *PrepareCommand) { c.SizeBytes = MaxImageUploadBytes + 1 }, func(c *PrepareCommand) { c.ContentType = "image/svg+xml" },
		func(c *PrepareCommand) { c.ContentType = "video/mp4"; c.SizeBytes = MaxUploadBytes + 1 },
	} {
		s := &prepareStorage{fakeStorage: &fakeStorage{}}
		c := validPrepare()
		change(&c)
		if _, err := New(s).Prepare(context.Background(), c); err == nil || s.calls != 0 {
			t.Fatal("invalid manifest reached storage", err)
		}
	}
}
