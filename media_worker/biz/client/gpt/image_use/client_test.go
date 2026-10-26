package image_use

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"media_agent/media_worker/biz/domain/gpt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Client, Config, string) {
	t.Helper()
	base := filepath.Join(t.TempDir(), "private")
	root := filepath.Join(base, "namespace", "image-use")
	script := filepath.Join(t.TempDir(), "bridge.py")
	key := strings.Repeat("a", 64)
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(imageBytes.Bytes())
	sha := hex.EncodeToString(sum[:])
	code := fmt.Sprintf(`import sys,os,json,hashlib,time
from pathlib import Path
args=sys.argv
assert args[args.index('--backend')+1]=='web'
assert 'MEDIA_WORKER_API_TOKEN' not in os.environ
assert 'private prompt' not in args
req=json.load(sys.stdin)
action=args[1];root=Path(args[args.index('--root')+1]);key=req.get('key','')
if action=='preflight' and (root/'slow').exists(): time.sleep(30)
out={'ok':True}
if action=='lookup': out.update(state='absent')
if action=='preflight': out.update(ready=True)
if action=='submit':
 assert req['prompt']=='private prompt'
 folder=root/key;folder.mkdir(mode=0o700,exist_ok=True)
 import base64
 p=folder/'output.png';p.write_bytes(base64.b64decode(%q));p.chmod(0o600)
 out.update(reference=key)
if action in ['observe','artifact']:
 out.update(state='succeeded',path=str(root/key/'output.png'),sha256=%q,size=%d,artifact_id='image-'+%q,media_type='image/png')
print(json.dumps(out))
`, base64.StdEncoding.EncodeToString(imageBytes.Bytes()), sha, imageBytes.Len(), sha)
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Command: []string{"/usr/bin/python3", script}, Root: root, InputRoot: filepath.Join(base, "namespace", "inputs"), LockDirectory: filepath.Join(base, "browser"), Upstream: "/private/pinned/image-use", Size: "auto", WebModel: "Thinking", Timeout: time.Minute}
	t.Setenv("MEDIA_WORKER_API_TOKEN", "must-not-reach-child")
	c, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, cfg, key
}
func TestWebOnlyTransportAndExactArtifactValidation(t *testing.T) {
	c, cfg, key := fixture(t)
	ctx := context.Background()
	lookup, err := c.Lookup(ctx, key)
	if err != nil || lookup.State != gpt.Absent {
		t.Fatal(lookup, err)
	}
	ref, err := c.Submit(ctx, gpt.Request{Key: key, Prompt: "private prompt", Size: "auto", WebModel: "Thinking"})
	if err != nil {
		t.Fatal(err)
	}
	observed, err := c.Observe(ctx, ref)
	if err != nil || observed.State != gpt.Succeeded {
		t.Fatal(observed, err)
	}
	files, err := c.FetchArtifacts(ctx, ref, observed.Artifacts)
	if err != nil || len(files) != 1 || files[0].SizeBytes <= 0 {
		t.Fatal(files, err)
	}
	if _, err = c.FetchArtifacts(ctx, ref, []gpt.Artifact{{ID: "foreign-artifact"}}); err == nil {
		t.Fatal("wrong artifact accepted")
	}
	if err = os.WriteFile(filepath.Join(cfg.Root, key, "output.png"), []byte("not the confirmed image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Observe(ctx, ref); err == nil {
		t.Fatal("changed media accepted")
	}
}
func TestGlobalBrowserLockAndSerialAdmission(t *testing.T) {
	c, cfg, _ := fixture(t)
	other := cfg
	other.Root = filepath.Join(filepath.Dir(cfg.Root), "other-namespace")
	if instance, err := Open(context.Background(), other); err == nil {
		instance.Close()
		t.Fatal("two workers own one Chrome")
	}
	release, err := c.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err = c.AcquireRecovery(ctx); err == nil {
		t.Fatal("second admission passed occupied slot")
	}
}
func TestBridgeCancellationDoesNotWaitForGenerationBudget(t *testing.T) {
	c, cfg, _ := fixture(t)
	if err := os.WriteFile(filepath.Join(cfg.Root, "slow"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Acquire(ctx); err == nil {
		t.Fatal("cancelled readiness accepted")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("child process ignored cancellation")
	}
}
func TestRejectUnsafeArtifactSymlink(t *testing.T) {
	c, cfg, key := fixture(t)
	ctx := context.Background()
	ref, err := c.Submit(ctx, gpt.Request{Key: key, Prompt: "private prompt"})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfg.Root, key, "output.png")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(cfg.InputRoot, "other.png")
	if err = os.WriteFile(other, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(p)
	if err = os.Symlink(other, p); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Observe(ctx, ref); err == nil {
		t.Fatal("symlink media accepted")
	}
}
