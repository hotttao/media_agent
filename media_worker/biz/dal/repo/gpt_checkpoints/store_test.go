package gpt_checkpoints

import (
	"context"
	"media_agent/media_worker/biz/domain/gpt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkpoint() gpt.Checkpoint {
	return gpt.Checkpoint{DispatchID: "dispatch", Attempt: 1, CapabilityKey: gpt.TextCapability, CapabilityVersion: 1, Request: gpt.Request{Key: strings.Repeat("a", 64), Prompt: "private prompt", Size: "auto", WebModel: "Thinking"}}
}
func TestImmutableGPTFactsAndIndependentStore(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "checkpoints"), 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "checkpoints", "corrupt-studio.json"), []byte("broken studio state"), 0600)
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c := checkpoint()
	if err = s.Save(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.Reference = gpt.Reference(c.Request.Key)
	c.Artifacts = []gpt.UploadedArtifact{{Download: gpt.Download{ArtifactID: "image", LocalPath: "/private/image.png", ChecksumSHA256: strings.Repeat("b", 64), SizeBytes: 42}}}
	if err = s.Save(ctx, c); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*gpt.Checkpoint){func(v *gpt.Checkpoint) { v.Request.Prompt = "changed" }, func(v *gpt.Checkpoint) { v.Reference = "other" }, func(v *gpt.Checkpoint) { v.Artifacts[0].LocalPath = "other" }, func(v *gpt.Checkpoint) { v.CapabilityKey = gpt.ReferenceCapability }} {
		fresh, _ := s.Load(ctx, c.DispatchID, c.Attempt)
		mutate(fresh)
		if s.Save(ctx, *fresh) == nil {
			t.Fatal("immutable fact changed")
		}
	}
	c.Artifacts[0].ObjectPath = "materials/org/image.png"
	if err = s.Save(ctx, c); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Pending(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatal(pending, err)
	}
	c.Closed = true
	if err = s.Save(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.Closed = false
	if s.Save(ctx, c) == nil {
		t.Fatal("closed generation reopened")
	}
	pending, err = s.Pending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatal(pending, err)
	}
}
func TestCorruptOrRemovedGPTCheckpointIsNotAnEmptyQueue(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := checkpoint()
	ctx := context.Background()
	if err = s.Save(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.dir, filename(c.DispatchID, c.Attempt)), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Load(ctx, c.DispatchID, c.Attempt); err == nil {
		t.Fatal("corrupt state accepted")
	}
	if _, err = s.Pending(ctx); err == nil {
		t.Fatal("corruption hid pending execution")
	}
}
