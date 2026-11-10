package wx_channel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

func TestDownloadUsesAuthenticatedMCPAndConfinedImmutableSnapshot(t *testing.T) {
	for _, rel := range []string{"video.mp4", "../outside.mp4", "escape.mp4"} {
		t.Run(rel, func(t *testing.T) {
			downloads := t.TempDir()
			outside := filepath.Join(t.TempDir(), "outside.mp4")
			video := []byte("\x00\x00\x00\x18ftypmp42video-data")
			os.WriteFile(filepath.Join(downloads, "video.mp4"), video, 0600)
			os.WriteFile(outside, video, 0600)
			os.Symlink(outside, filepath.Join(downloads, "escape.mp4"))
			calls := 0
			server := mcp.NewServer(&mcp.Implementation{Name: "wx-fixture", Version: "1"}, nil)
			mcp.AddTool(server, &mcp.Tool{Name: "download_video_for_analysis"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct {
				RecordID string `json:"record_id"`
			}) (*mcp.CallToolResult, map[string]any, error) {
				calls++
				if in.RecordID != "record" {
					t.Error("record identity lost")
				}
				status := "in_progress"
				if calls > 1 {
					status = "completed"
				}
				return nil, map[string]any{"data": map[string]any{"status": status, "file": map[string]any{"exists": true, "relative_path": rel}}}, nil
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("missing authentication")
					w.WriteHeader(401)
					return
				}
				handler.ServeHTTP(w, r)
			}))
			defer httpServer.Close()
			token := filepath.Join(t.TempDir(), "token")
			os.WriteFile(token, []byte("test-secret\n"), 0600)
			ctx := context.Background()
			client, err := Open(ctx, httpServer.URL+"/mcp", token, downloads)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			target := filepath.Join(t.TempDir(), "snapshot.mp4")
			if _, err = client.Download(ctx, "record", target); !errors.Is(err, worker_runtime.ErrExecutionPending) {
				t.Fatal("async result was treated as complete", err)
			}
			path, err := client.Download(ctx, "record", target)
			if rel != "video.mp4" {
				if err == nil {
					t.Fatal("path escape accepted")
				}
				return
			}
			if err != nil || path != target {
				t.Fatal(path, err)
			}
			os.WriteFile(filepath.Join(downloads, "video.mp4"), []byte("changed"), 0600)
			if _, err = client.Download(ctx, "record", target); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(target)
			if string(got) != string(video) || calls != 2 {
				t.Fatal("snapshot changed during replay")
			}
		})
	}
}
func TestOpenRejectsRemoteCredentialDestinations(t *testing.T) {
	for _, endpoint := range []string{"http://example.com/mcp", "https://localhost/mcp", "http://localhost/mcp?secret=1", "http://user@localhost/mcp"} {
		if _, err := Open(context.Background(), endpoint, "unused", "unused"); err == nil {
			t.Fatal(endpoint)
		}
	}
}
