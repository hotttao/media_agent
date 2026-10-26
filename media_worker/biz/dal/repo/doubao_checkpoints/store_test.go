package doubao_checkpoints

import (
	"context"
	"media_agent/media_worker/biz/domain/doubao"
	"media_agent/media_worker/biz/shared/worker_runtime"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointRestoresWithoutInputAndRejectsRegressions(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cp := doubao.Checkpoint{Task: worker_runtime.Task{DispatchID: "dispatch", Attempt: 1, CapabilityKey: "cap", CapabilityVersion: 1, Input: map[string]any{"images": []string{"private-signed-url"}}}, GenerationKey: "key", RequestDigest: "digest", Intent: doubao.GenerationIntent{Prompt: "rendered"}}
	if err := s.Save(ctx, cp); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(ctx, "dispatch", 1)
	if err != nil || loaded.Task.Input != nil || loaded.Intent.Prompt != "rendered" {
		t.Fatal(loaded, err)
	}
	changed := *loaded
	changed.Intent.Prompt = "different"
	if reopened.Save(ctx, changed) == nil {
		t.Fatal("intent overwritten")
	}
	loaded.Reference = "exact-reference"
	if reopened.Save(ctx, *loaded) != nil {
		t.Fatal("bind failed")
	}
	loaded.Reference = "other"
	if reopened.Save(ctx, *loaded) == nil {
		t.Fatal("reference overwritten")
	}
	loaded.Reference = "exact-reference"
	loaded.Closed = true
	if reopened.Save(ctx, *loaded) != nil {
		t.Fatal("close failed")
	}
	loaded.Closed = false
	if reopened.Save(ctx, *loaded) == nil {
		t.Fatal("closed checkpoint reopened")
	}
	pending, err := reopened.Pending(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatal("closed checkpoint recovered", err)
	}
}
func TestCheckpointCorruptionFailsClosed(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if os.WriteFile(filepath.Join(s.dir, "bad.json"), []byte(`{"task":{"dispatch_id":"missing"}}`), 0600) != nil {
		t.Fatal("write")
	}
	if _, err := s.Pending(context.Background()); err == nil {
		t.Fatal("corrupt checkpoint ignored")
	}
}
