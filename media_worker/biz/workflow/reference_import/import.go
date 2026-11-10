// Package reference_import coordinates client-side reference acquisition and
// authorized artifact delivery; it never handles WeChat session credentials.
package reference_import

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"media_agent/media_worker/biz/shared/worker_runtime"
)

const Capability = "wx_channel.download.record"

type Source interface {
	Download(context.Context, string, string) (string, error)
}
type Workflow struct {
	Source         Source
	Gateway        worker_runtime.Gateway
	Uploader       worker_runtime.Uploader
	WorkerID, Root string
}

func (w *Workflow) AvailableCapabilities() []worker_runtime.Capability {
	return []worker_runtime.Capability{{Key: Capability, Version: 1, Concurrency: 1}}
}
func (w *Workflow) ExecuteContext(ctx context.Context, t worker_runtime.Task) (map[string]any, error) {
	if t.CapabilityKey != Capability || t.CapabilityVersion != 1 {
		return nil, errors.New("unsupported reference import capability")
	}
	id, ok := t.Input["record_id"].(string)
	if !ok || strings.TrimSpace(id) == "" || len(id) > 512 {
		return nil, errors.New("wx_channel browse record_id required")
	}
	if err := w.Gateway.Heartbeat(ctx, w.WorkerID, t); err != nil {
		if errors.Is(err, worker_runtime.ErrResultRejected) {
			return nil, worker_runtime.ErrExecutionAbandoned
		}
		return nil, worker_runtime.ErrExecutionPending
	}
	key := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", t.DispatchID+":"+id, t.Attempt)))
	file, err := w.Source.Download(ctx, id, filepath.Join(w.Root, hex.EncodeToString(key[:]), "reference.mp4"))
	if err != nil {
		return nil, err
	}
	// Recheck the lease after a potentially slow local copy and before upload.
	if err = w.Gateway.Heartbeat(ctx, w.WorkerID, t); err != nil {
		if errors.Is(err, worker_runtime.ErrResultRejected) {
			return nil, worker_runtime.ErrExecutionAbandoned
		}
		return nil, worker_runtime.ErrExecutionPending
	}
	object, err := w.Uploader.UploadFile(ctx, w.WorkerID, t, 0, file)
	if err != nil || object == "" {
		if errors.Is(err, worker_runtime.ErrResultRejected) {
			return nil, worker_runtime.ErrExecutionAbandoned
		}
		return nil, worker_runtime.ErrExecutionPending
	}
	return map[string]any{"candidates": []map[string]any{{"index": 0, "object_path": object, "content": "", "is_primary": true}}, "provenance": map[string]any{"provider": "wx_channel", "record_id": id, "product_information_status": "unsupported"}}, nil
}
