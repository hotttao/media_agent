// Package gpt_generate coordinates browser image generation. Studio is unrelated.
package gpt_generate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"media_agent/media_worker/biz/domain/gpt"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

type Workflow struct {
	Generator                           gpt.Generator
	Checkpoints                         gpt.Repository
	Gateway                             worker_runtime.Gateway
	Uploader                            worker_runtime.Uploader
	Inputs                              worker_runtime.InputDownloader
	Files                               worker_runtime.ArtifactVerifier
	WorkerID, Namespace, Size, WebModel string
}

func (w *Workflow) AvailableCapabilities() []worker_runtime.Capability {
	return []worker_runtime.Capability{{Key: gpt.TextCapability, Version: 1, Concurrency: 1}, {Key: gpt.ReferenceCapability, Version: 1, Concurrency: 1}}
}
func stableKey(v ...any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (w *Workflow) authorize(ctx context.Context, t worker_runtime.Task) error {
	if err := w.Gateway.Heartbeat(ctx, w.WorkerID, t); err != nil {
		if errors.Is(err, worker_runtime.ErrResultRejected) {
			if w.FinishTask(ctx, t) != nil {
				return worker_runtime.ErrExecutionPending
			}
			return worker_runtime.ErrExecutionAbandoned
		}
		return worker_runtime.ErrExecutionPending
	}
	return nil
}
func (w *Workflow) ExecuteContext(ctx context.Context, t worker_runtime.Task) (map[string]any, error) {
	if t.CapabilityVersion != 1 || (t.CapabilityKey != gpt.TextCapability && t.CapabilityKey != gpt.ReferenceCapability) {
		return nil, errors.New("unsupported GPT image capability")
	}
	if err := w.authorize(ctx, t); err != nil {
		return nil, err
	}
	ctx, stop := w.whileEligible(ctx, t)
	defer stop()
	cp, err := w.Checkpoints.Load(ctx, t.DispatchID, t.Attempt)
	if err != nil {
		return nil, worker_runtime.ErrExecutionPending
	}
	if cp != nil && cp.Closed {
		return nil, worker_runtime.ErrExecutionAbandoned
	}
	if cp == nil {
		prompt, ok := t.Input["prompt"].(string)
		if !ok || strings.TrimSpace(prompt) == "" || len(prompt) > 48000 {
			return nil, errors.New("rendered GPT prompt required")
		}
		var refs []string
		switch values := t.Input["images"].(type) {
		case nil:
		case []string:
			refs = values
		case []any:
			for _, v := range values {
				s, ok := v.(string)
				if !ok {
					return nil, errors.New("GPT references must be strings")
				}
				refs = append(refs, s)
			}
		default:
			return nil, errors.New("GPT references must be a list")
		}
		if (t.CapabilityKey == gpt.TextCapability && len(refs) != 0) || (t.CapabilityKey == gpt.ReferenceCapability && (len(refs) < 1 || len(refs) > 4)) {
			return nil, errors.New("GPT reference count does not match capability")
		}
		key := stableKey(w.Namespace, t.DispatchID, t.Attempt, "gpt_image_use")
		request := gpt.Request{Key: key, Prompt: prompt, Size: w.Size, WebModel: w.WebModel}
		for i, raw := range refs {
			image, err := w.Inputs.Fetch(ctx, stableKey(key, i), raw)
			if err != nil {
				return nil, worker_runtime.ErrExecutionPending
			}
			request.Images = append(request.Images, gpt.LocalImage{Path: image.LocalPath, ChecksumSHA256: image.ChecksumSHA256, SizeBytes: image.SizeBytes})
		}
		cp = &gpt.Checkpoint{DispatchID: t.DispatchID, Attempt: t.Attempt, CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion, TraceParent: t.TraceParent, Request: request}
		if w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, worker_runtime.ErrExecutionPending
		}
	}
	if cp.CapabilityKey != t.CapabilityKey || cp.CapabilityVersion != t.CapabilityVersion {
		return nil, worker_runtime.ErrExecutionPending
	}
	if cp.Reference == "" {
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		found, err := w.Generator.Lookup(ctx, cp.Request.Key)
		if err != nil {
			return nil, worker_runtime.ErrExecutionPending
		}
		switch found.State {
		case gpt.Found:
			cp.Reference = found.Reference
		case gpt.Absent:
			for _, i := range cp.Request.Images {
				if w.Files == nil || w.Files.Verify(ctx, worker_runtime.Artifact{LocalPath: i.Path, ChecksumSHA256: i.ChecksumSHA256, SizeBytes: i.SizeBytes}) != nil {
					return nil, worker_runtime.ErrExecutionPending
				}
			}
			ref, err := w.Generator.Submit(ctx, cp.Request)
			if err != nil {
				return nil, worker_runtime.ErrExecutionPending
			}
			cp.Reference = ref
		default:
			return nil, worker_runtime.ErrExecutionPending
		}
		if cp.Reference == "" || w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, worker_runtime.ErrExecutionPending
		}
	}
	if len(cp.Artifacts) == 0 {
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		observed, err := w.Generator.Observe(ctx, cp.Reference)
		if err != nil || observed.State != gpt.Succeeded || len(observed.Artifacts) != 1 || !strings.HasPrefix(observed.Artifacts[0].MediaType, "image/") {
			return nil, worker_runtime.ErrExecutionPending
		}
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		files, err := w.Generator.FetchArtifacts(ctx, cp.Reference, observed.Artifacts)
		if err != nil || len(files) != 1 || files[0].ArtifactID != observed.Artifacts[0].ID || files[0].LocalPath == "" {
			return nil, worker_runtime.ErrExecutionPending
		}
		cp.Artifacts = []gpt.UploadedArtifact{{Download: files[0]}}
		if w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, worker_runtime.ErrExecutionPending
		}
	}
	if len(cp.Artifacts) != 1 {
		return nil, worker_runtime.ErrExecutionPending
	}
	a := &cp.Artifacts[0]
	if a.ObjectPath == "" {
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		if w.Files == nil || w.Files.Verify(ctx, worker_runtime.Artifact{ID: a.ArtifactID, LocalPath: a.LocalPath, ChecksumSHA256: a.ChecksumSHA256, SizeBytes: a.SizeBytes}) != nil {
			return nil, worker_runtime.ErrExecutionPending
		}
		object, err := w.Uploader.UploadFile(ctx, w.WorkerID, t, 0, a.LocalPath)
		if err != nil || object == "" {
			return nil, worker_runtime.ErrExecutionPending
		}
		a.ObjectPath = object
		if w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, worker_runtime.ErrExecutionPending
		}
	}
	return map[string]any{"candidates": []map[string]any{{"index": 0, "object_path": a.ObjectPath, "is_primary": true}}, "provenance": map[string]any{"provider": "gpt", "profile": "gpt_image_use", "backend": "web", "mock": false, "generation_key": cp.Request.Key}}, nil
}
func (w *Workflow) PendingTasks(ctx context.Context) ([]worker_runtime.Task, error) {
	cps, err := w.Checkpoints.Pending(ctx)
	if err != nil {
		return nil, err
	}
	var tasks []worker_runtime.Task
	for _, c := range cps {
		tasks = append(tasks, worker_runtime.Task{DispatchID: c.DispatchID, Attempt: c.Attempt, CapabilityKey: c.CapabilityKey, CapabilityVersion: c.CapabilityVersion, TraceParent: c.TraceParent})
	}
	return tasks, nil
}
func (w *Workflow) FinishTask(ctx context.Context, t worker_runtime.Task) error {
	c, err := w.Checkpoints.Load(ctx, t.DispatchID, t.Attempt)
	if err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	c.Closed = true
	if err = w.Checkpoints.Save(ctx, *c); err != nil {
		return fmt.Errorf("close GPT checkpoint: %w", err)
	}
	return nil
}

var _ worker_runtime.Executor = (*Workflow)(nil)
var _ worker_runtime.ExecutionRecovery = (*Workflow)(nil)

// A CLI call may block for minutes. Stop our process when current eligibility
// cannot be confirmed; recovery still uses the original receipt and never sends again.
func (w *Workflow) whileEligible(parent context.Context, t worker_runtime.Task) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if w.Gateway.Heartbeat(ctx, w.WorkerID, t) != nil {
					cancel()
					return
				}
			}
		}
	}()
	return ctx, func() { cancel(); <-done }
}
