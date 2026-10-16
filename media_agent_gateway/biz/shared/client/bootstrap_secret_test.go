package client

import (
	"bytes"
	"context"
	"media_agent/hertz_infra/sessioncredential"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBootstrapFilesReuseEncryptAndRejectTampering(t *testing.T) {
	keys, err := sessioncredential.New("k1", map[string][]byte{"k1": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	store := BootstrapFiles{Root: filepath.Join(t.TempDir(), "private"), Keys: keys}
	var calls atomic.Int32
	create := func() (BootstrapCredential, error) {
		calls.Add(1)
		return BootstrapCredential{Secret: "private-agent-material", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := store.GetOrCreate(context.Background(), "identity", create)
			if e != nil || v.Secret != "private-agent-material" {
				t.Errorf("bootstrap failed: %v", e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("issued %d times", calls.Load())
	}
	entries, err := os.ReadDir(store.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".lock" {
			continue
		}
		p := filepath.Join(store.Root, entry.Name())
		data, err := os.ReadFile(p)
		if err != nil || bytes.Contains(data, []byte("private-agent-material")) {
			t.Fatal("plaintext stored")
		}
		info, _ := os.Stat(p)
		if info.Mode().Perm() != 0600 {
			t.Fatal("secret permissions")
		}
		data[len(data)/2] ^= 1
		if err = os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.GetOrCreate(context.Background(), "identity", create); err == nil || calls.Load() != 1 {
		t.Fatal("corrupt secret silently replaced")
	}
	if _, err = store.GetOrCreate(context.Background(), "other-identity", create); err != nil || calls.Load() != 2 {
		t.Fatal("identities share credential")
	}
}
