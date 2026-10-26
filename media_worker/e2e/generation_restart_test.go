package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"media_agent/media_worker/biz/client/doubao/studio"
	"media_agent/media_worker/biz/client/media_input"
	"media_agent/media_worker/biz/client/worker_gateway"
	"media_agent/media_worker/biz/dal/repo/doubao_checkpoints"
	"media_agent/media_worker/biz/dal/repo/results"
	"media_agent/media_worker/biz/domain/doubao"
	"media_agent/media_worker/biz/shared/worker_runtime"
	"media_agent/media_worker/biz/workflow/doubao_generate"
	"media_agent/media_worker/biz/workflow/execute"
)

type exitAfterCommit struct {
	store        *doubao_checkpoints.Store
	after, count int
}

func (s *exitAfterCommit) Save(ctx context.Context, c doubao.Checkpoint) error {
	if err := s.store.Save(ctx, c); err != nil {
		return err
	}
	s.count++
	if s.after > 0 && s.count == s.after {
		os.Exit(77)
	}
	return nil
}
func (s *exitAfterCommit) Load(ctx context.Context, d string, a int) (*doubao.Checkpoint, error) {
	return s.store.Load(ctx, d, a)
}
func (s *exitAfterCommit) Pending(ctx context.Context) ([]doubao.Checkpoint, error) {
	return s.store.Pending(ctx)
}
func TestGenerationCheckpointProcessCrash(t *testing.T) {
	if os.Getenv("MEDIA_WORKER_GENERATION_CHILD") == "1" {
		endpoint := os.Getenv("MEDIA_WORKER_GENERATION_ENDPOINT")
		root := os.Getenv("MEDIA_WORKER_GENERATION_STATE")
		journal, err := results.Open(root, endpoint, "worker", "test-token")
		if err != nil {
			t.Fatal(err)
		}
		defer journal.Close()
		cp, err := doubao_checkpoints.Open(journal.Namespace())
		if err != nil {
			t.Fatal(err)
		}
		after, _ := strconv.Atoi(os.Getenv("MEDIA_WORKER_GENERATION_EXIT_COMMIT"))
		repo := &exitAfterCommit{store: cp, after: after}
		client, err := studio.NewClient(endpoint, "studio-token", "account", filepath.Join(journal.Namespace(), "artifacts"))
		if err != nil {
			t.Fatal(err)
		}
		gateway := worker_gateway.New(endpoint, "test-token")
		files := media_input.Downloader{Root: filepath.Join(journal.Namespace(), "inputs")}
		flow := &doubao_generate.Workflow{Generator: client, Gateway: gateway, Uploader: gateway, Checkpoints: repo, Inputs: files, Files: files, WorkerID: "worker", Namespace: filepath.Base(journal.Namespace()), Model: "seedance-2.0-fast", DurationSeconds: 5, AspectRatio: "9:16"}
		task := worker_runtime.Task{DispatchID: "dispatch", Attempt: 1, CapabilityKey: doubao_generate.CapabilityKey, CapabilityVersion: 1}
		if after > 0 {
			task.Input = map[string]any{"prompt": "rendered", "images": []any{endpoint + "/input.png?private-signed-sentinel"}}
		}
		output, err := flow.ExecuteContext(context.Background(), task)
		if err != nil {
			t.Fatal(err)
		}
		record := worker_runtime.Result{Task: task, Key: "dispatch:1:complete", Output: output}
		if err := journal.Save(record); err != nil {
			t.Fatal(err)
		}
		if ok, err := execute.DeliverStoredResult(context.Background(), gateway, "worker", journal, record); err != nil || !ok {
			t.Fatal(err)
		}
		if err := flow.FinishTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, phase := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(phase), func(t *testing.T) {
			var mu sync.Mutex
			created := false
			submits, completes := 0, 0
			image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==")
			video := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, make([]byte, 16)...)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/input.png":
					if r.Header.Get("Authorization") != "" {
						t.Error("input token leak")
					}
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write(image)
				case strings.HasSuffix(r.URL.Path, "/heartbeat"):
					_, _ = w.Write([]byte(`{}`))
				case strings.HasSuffix(r.URL.Path, "/uploads"):
					_, _ = w.Write([]byte(`{"already_uploaded":true,"object_path":"materials/org/result.mp4"}`))
				case strings.HasSuffix(r.URL.Path, "/complete"):
					completes++
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["idempotency_key"] != "dispatch:1:complete" {
						t.Error("delivery key changed")
					}
					_, _ = w.Write([]byte(`{"status":"submitted"}`))
				case strings.Contains(r.URL.Path, "/artifacts/exact-video"):
					w.Header().Set("Content-Type", "video/mp4")
					_, _ = w.Write(video)
				case strings.HasPrefix(r.URL.Path, "/v1/worker/generations/"):
					if r.Header.Get("Authorization") != "Bearer studio-token" {
						t.Error("gateway token reached studio")
					}
					key := strings.Split(r.URL.Path, "/")[4]
					if r.Method == "PUT" {
						submits++
						created = true
					}
					if !created {
						w.WriteHeader(404)
						_, _ = w.Write([]byte(`{"code":"WORKER_NOT_FOUND","authoritative":true}`))
						return
					}
					fmt.Fprintf(w, `{"ok":true,"reference":"%s","status":"done","artifacts":[{"id":"exact-video","kind":"VIDEO","mediaType":"video/mp4"}]}`, key)
				default:
					t.Errorf("unexpected route %s", r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			state := filepath.Join(t.TempDir(), "private")
			child := func(commit int) error {
				cmd := exec.Command(os.Args[0], "-test.run=^TestGenerationCheckpointProcessCrash$")
				cmd.Env = append(os.Environ(), "MEDIA_WORKER_GENERATION_CHILD=1", "MEDIA_WORKER_GENERATION_ENDPOINT="+server.URL, "MEDIA_WORKER_GENERATION_STATE="+state, "MEDIA_WORKER_GENERATION_EXIT_COMMIT="+strconv.Itoa(commit))
				out, err := cmd.CombinedOutput()
				if commit == 0 && err != nil {
					t.Fatalf("resume child: %s %v", out, err)
				}
				return err
			}
			err := child(phase)
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 77 {
				t.Fatalf("did not crash after commit %d: %v", phase, err)
			}
			_ = child(0)
			mu.Lock()
			gotSubmits, gotCompletes := submits, completes
			mu.Unlock()
			if gotSubmits != 1 || gotCompletes != 1 {
				t.Fatalf("submissions=%d completions=%d", gotSubmits, gotCompletes)
			}
			_ = filepath.WalkDir(state, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(path, ".json") {
					data, _ := os.ReadFile(path)
					if strings.Contains(string(data), "private-signed-sentinel") {
						t.Error("signed URL persisted")
					}
				}
				return nil
			})
		})
	}
}
