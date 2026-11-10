package reference_import

import (
	"context"
	"errors"
	"media_agent/media_worker/biz/shared/worker_runtime"
	"testing"
)

type sourceFake struct {
	calls int
	err   error
}

func (s *sourceFake) Download(context.Context, string, string) (string, error) {
	s.calls++
	return "snapshot.mp4", s.err
}

type gatewayFake struct {
	worker_runtime.Gateway
	beats, uploads int
	rejectAt       int
	uploadErr      error
}

func (g *gatewayFake) Heartbeat(context.Context, string, worker_runtime.Task) error {
	g.beats++
	if g.beats == g.rejectAt {
		return worker_runtime.ErrResultRejected
	}
	return nil
}
func (g *gatewayFake) UploadFile(context.Context, string, worker_runtime.Task, int, string) (string, error) {
	g.uploads++
	return "references/video.mp4", g.uploadErr
}
func TestImportRequiresEligibilityAndConfirmedUpload(t *testing.T) {
	for _, kind := range []string{"success", "pending", "rejected_before", "rejected_after", "upload_unknown", "invalid_input"} {
		t.Run(kind, func(t *testing.T) {
			g := &gatewayFake{}
			s := &sourceFake{}
			task := worker_runtime.Task{DispatchID: "dispatch", Attempt: 1, CapabilityKey: Capability, CapabilityVersion: 1, Input: map[string]any{"record_id": "browse-id"}}
			switch kind {
			case "pending":
				s.err = worker_runtime.ErrExecutionPending
			case "rejected_before":
				g.rejectAt = 1
			case "rejected_after":
				g.rejectAt = 2
			case "upload_unknown":
				g.uploadErr = errors.New("lost upload response")
			case "invalid_input":
				task.Input = map[string]any{"url": "https://weixin.qq.com/sph/1"}
			}
			got, err := (&Workflow{Source: s, Gateway: g, Uploader: g, WorkerID: "worker", Root: t.TempDir()}).ExecuteContext(context.Background(), task)
			switch kind {
			case "success":
				if err != nil || got["candidates"] == nil || g.uploads != 1 {
					t.Fatal(got, err)
				}
			case "pending", "upload_unknown":
				if !errors.Is(err, worker_runtime.ErrExecutionPending) || got != nil {
					t.Fatal("unconfirmed operation completed", got, err)
				}
			case "rejected_before", "rejected_after":
				if !errors.Is(err, worker_runtime.ErrExecutionAbandoned) || g.uploads != 0 {
					t.Fatal("stale lease executed", err)
				}
			case "invalid_input":
				if err == nil || s.calls != 0 || g.beats != 0 {
					t.Fatal("raw URL accepted")
				}
			}
		})
	}
}
