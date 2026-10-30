package studio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"media_agent/media_worker/biz/domain/doubao"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLookupRequiresAuthoritativeNegative(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		state  doubao.LookupState
	}{{404, `{"code":"NOT_FOUND"}`, doubao.Uncertain}, {503, `{"code":"TIMEOUT"}`, doubao.Uncertain}, {404, `{"code":"WORKER_NOT_FOUND","authoritative":true}`, doubao.Absent}, {200, fmt.Sprintf(`{"ok":true,"reference":"%s"}`, strings.Repeat("a", 64)), doubao.Found}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer local-token" {
				t.Error("missing local identity")
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		client, err := NewClient(server.URL, "local-token", "account", t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.Lookup(context.Background(), strings.Repeat("a", 64))
		server.Close()
		if result.State != tc.state || (tc.state == doubao.Uncertain && err == nil) {
			t.Fatalf("lookup state=%v err=%v", result, err)
		}
	}
}
func TestSubmitAndFetchUseStableKeyAndExactArtifact(t *testing.T) {
	key := strings.Repeat("a", 64)
	var calls []string
	payload := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, make([]byte, 16)...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == "PUT" {
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["accountId"] != "account" || body["duration"] != "5s" || body["model"] != "seedance-2.0-fast" {
				t.Error("generation parameters changed")
			}
			fmt.Fprintf(w, `{"ok":true,"reference":"%s"}`, key)
			return
		}
		if r.URL.Path != "/v1/worker/generations/"+key+"/artifacts/exact-video" {
			t.Error("wrong artifact binding")
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "local-token", "account", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := client.Submit(context.Background(), doubao.Request{Key: key, Kind: doubao.Video, Prompt: "rendered", Images: []doubao.LocalImage{{Path: "/private/input.png"}}, Model: "seedance-2.0-fast", DurationSeconds: 5, AspectRatio: "9:16"})
	if err != nil || string(ref) != key {
		t.Fatal(ref, err)
	}
	files, err := client.FetchArtifacts(context.Background(), ref, []doubao.Artifact{{ID: "exact-video", Kind: doubao.Video}})
	if err != nil || len(files) != 1 || files[0].ArtifactID != "exact-video" || files[0].SizeBytes != int64(len(payload)) || len(files[0].ChecksumSHA256) != 64 {
		t.Fatal(files, err)
	}
	info, err := os.Stat(files[0].LocalPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("artifact not private", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(files[0].LocalPath))
	if len(entries) != 1 || len(calls) != 2 {
		t.Fatal("partial file or extra request")
	}
}
func TestDedicatedProfileRejectsConcurrentOwners(t *testing.T) {
	cfg := InstanceConfig{UserData: filepath.Join(t.TempDir(), "profile"), AssetRoot: t.TempDir(), AccountID: "account"}
	first, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := Open(cfg); err == nil {
		second.Close()
		t.Fatal("two workers own same studio profile")
	}
}

func TestDiscoveryUsesPrivateIdentityProofInsteadOfProcessEnvironment(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	assets := filepath.Join(root, "assets")
	if err := os.Mkdir(assets, 0700); err != nil {
		t.Fatal(err)
	}
	instance, err := Open(InstanceConfig{UserData: profile, AssetRoot: assets, AccountID: "account", ArtifactRoot: filepath.Join(root, "artifacts")})
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	digest := func(value string) string { sum := sha256.Sum256([]byte(value)); return hex.EncodeToString(sum[:]) }
	profileHash, assetHash := digest(profile), digest(assets)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-proof" {
			t.Error("private proof not used")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": true, "rendererReady": true, "pid": os.Getpid(), "workerProtocol": "worker-v1", "profileHash": profileHash, "assetRootHash": assetHash})
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	control := filepath.Join(profile, "DoubaoStudioControl")
	if os.Mkdir(control, 0700) != nil {
		t.Fatal("control directory")
	}
	info := discoveryDTO{Address: "127.0.0.1", Port: port, PID: os.Getpid(), TokenFile: "control-token", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), ProfileHash: profileHash, AssetRootHash: assetHash}
	write := func() {
		raw, _ := json.Marshal(info)
		if os.WriteFile(filepath.Join(control, "control-info.json"), raw, 0600) != nil {
			t.Fatal("info")
		}
	}
	write()
	if os.WriteFile(filepath.Join(control, "control-token"), []byte("local-proof"), 0600) != nil {
		t.Fatal("token")
	}
	if err := instance.discover(context.Background()); err != nil {
		t.Fatalf("valid proof depended on /proc environ: %v", err)
	}
	info.AssetRootHash = strings.Repeat("0", 64)
	write()
	if instance.discover(context.Background()) == nil {
		t.Fatal("adopted another credential namespace")
	}
}
func TestAdmissionRefusesBusyProviderBeforeCheckingQuota(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/health" {
			t.Error("busy provider reached account/poll path")
		}
		_, _ = w.Write([]byte(`{"workerBusy":true}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "token", "account", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.AccountReady(context.Background()) == nil || calls != 1 {
		t.Fatal("busy provider admitted")
	}
}

func TestSubmitPreservesAllOrderedImagePaths(t *testing.T) {
	key := strings.Repeat("b", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Images []string `json:"images"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Images) != 2 || body.Images[0] != "/first.png" || body.Images[1] != "/second.png" {
			t.Error("ordered input images lost")
		}
		fmt.Fprintf(w, `{"ok":true,"reference":"%s"}`, key)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "token", "account", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Submit(context.Background(), doubao.Request{Key: key, Kind: doubao.Video, Prompt: "prompt", Images: []doubao.LocalImage{{Path: "/first.png"}, {Path: "/second.png"}}, Model: "seedance-2.0-fast", DurationSeconds: 5, AspectRatio: "9:16"})
	if err != nil {
		t.Fatal(err)
	}
}
