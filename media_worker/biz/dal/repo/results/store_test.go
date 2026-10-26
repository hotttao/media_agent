package results

import (
	"os"
	"path/filepath"
	"testing"

	"media_agent/media_worker/biz/domain/execution"
)

func TestResultSurvivesRestartAndCannotBeChangedOrCrossCredentialReplayed(t *testing.T) {
	base := filepath.Join(t.TempDir(), "private")
	store, err := Open(base, "http://gateway", "worker", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"
	r := execution.Result{Task: execution.Task{DispatchID: "dispatch", Attempt: 1, Input: map[string]any{"prompt": "private prompt"}, TraceParent: parent}, Key: "stable", Output: map[string]any{"candidates": []any{map[string]any{"content": "result"}}}}
	if store.Save(r) != nil || store.Save(r) != nil {
		t.Fatal("same result not saved idempotently")
	}
	if _, err := Open(base, "http://gateway", "worker", "test-token"); err == nil {
		t.Fatal("concurrent client acquired result ownership")
	}
	rChanged := r
	rChanged.Output = map[string]any{"different": "result"}
	if store.Save(rChanged) == nil {
		t.Fatal("saved result was overwritten")
	}
	store.Close()
	store, err = Open(base, "http://gateway", "worker", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pending, err := store.Pending()
	if err != nil || len(pending) != 1 || len(pending[0].Task.Input) != 0 || pending[0].Key != "stable" {
		t.Fatal("result was not recovered without prompt duplication")
	}
	if pending[0].Task.TraceParent != parent {
		t.Fatal("result replay lost original execution trace")
	}
	other, err := Open(base, "http://gateway", "worker", "another-token")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	foreign, err := other.Pending()
	if err != nil || len(foreign) != 0 {
		t.Fatal("credential namespace crossed")
	}
	if store.Finish(pending[0], true) != nil {
		t.Fatal("rejected result not retained")
	}
	pending, err = store.Pending()
	if err != nil || len(pending) != 0 {
		t.Fatal("rejected result will replay")
	}
}

func TestPrivateStoreRejectsUnsafeDirectoryAndCorruptRecords(t *testing.T) {
	base := filepath.Join(t.TempDir(), "public")
	if err := os.Mkdir(base, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(base, "http://gateway", "worker", "token"); err == nil {
		t.Fatal("public result directory accepted")
	}
	base = filepath.Join(t.TempDir(), "private")
	store, err := Open(base, "http://gateway", "worker", "token")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := os.WriteFile(filepath.Join(store.dir, "bad.json"), []byte("private malformed payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pending(); err != ErrStorage {
		t.Fatal("corrupt record was skipped or payload leaked")
	}
}

// Literal emitted by the original internal/results implementation. No new DTO
// is used to create this fixture, so field-name changes cannot mask breakage.
func TestLegacyLiteralRestoresAndResavesIdentically(t *testing.T) {
	const old = `{"task":{"dispatch_id":"legacy","attempt":2,"capability_key":"cap","capability_version":1,"input_json":"","lease_expires_at":123,"trace_parent":"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00"},"key":"legacy:2:complete","output":{"candidates":[{"content":"saved"}]}}`
	store, err := Open(filepath.Join(t.TempDir(), "private"), "http://gateway/", "worker", "token")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	expectedNamespace := hash("http://gateway\x00worker\x00token")
	if filepath.Base(store.dir) != expectedNamespace {
		t.Fatal("legacy namespace changed")
	}
	filename := filepath.Join(store.dir, hash("legacy\x00legacy:2:complete")+".json")
	if err := os.WriteFile(filename, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Pending()
	if err != nil || len(pending) != 1 || pending[0].Key != "legacy:2:complete" || pending[0].Task.Attempt != 2 {
		t.Fatalf("legacy restore: %v %v", pending, err)
	}
	if err := store.Save(pending[0]); err != nil {
		t.Fatalf("legacy result cannot be saved idempotently: %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != old {
		t.Fatal("old disk format rewritten")
	}
	if err := store.Finish(pending[0], true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filename + ".rejected"); err != nil {
		t.Fatal("rejected result not retained")
	}
}
