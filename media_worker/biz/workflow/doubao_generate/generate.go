// Package doubao_generate coordinates execution eligibility, generation facts and artifact delivery.
package doubao_generate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"media_agent/media_worker/biz/domain/doubao"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

const CapabilityKey = "doubao.i2v.first_frame_prompt"

type Workflow struct {
	Files                                   worker_runtime.ArtifactVerifier
	Generator                               doubao.Generator
	Gateway                                 worker_runtime.Gateway
	Uploader                                worker_runtime.Uploader
	Checkpoints                             doubao.CheckpointRepository
	Inputs                                  worker_runtime.InputDownloader
	WorkerID, Namespace, Model, AspectRatio string
	DurationSeconds                         int
}

func (w *Workflow) AvailableCapabilities() []worker_runtime.Capability {
	return []worker_runtime.Capability{{Key: CapabilityKey, Version: 1, Concurrency: 1}}
}
func pending() error { return worker_runtime.ErrExecutionPending }
func (w *Workflow) authorize(ctx context.Context, t worker_runtime.Task) error {
	if err := w.Gateway.Heartbeat(ctx, w.WorkerID, t); err != nil {
		if errors.Is(err, worker_runtime.ErrResultRejected) {
			if err := w.FinishTask(ctx, t); err != nil {
				return pending()
			}
			return worker_runtime.ErrExecutionAbandoned
		}
		return pending()
	}
	return nil
}
func (w *Workflow) ExecuteContext(ctx context.Context, t worker_runtime.Task) (map[string]any, error) {
	if t.CapabilityKey != CapabilityKey || t.CapabilityVersion != 1 {
		return nil, errors.New("unsupported production capability")
	}
	if err := w.authorize(ctx, t); err != nil {
		return nil, err
	}
	cp, err := w.Checkpoints.Load(ctx, t.DispatchID, t.Attempt)
	if err != nil {
		return nil, pending()
	}
	if cp != nil && cp.Closed {
		return nil, worker_runtime.ErrExecutionAbandoned
	}
	if cp == nil {
		prompt, ok := t.Input["prompt"].(string)
		if !ok || strings.TrimSpace(prompt) == "" {
			return nil, errors.New("rendered prompt required")
		}
		var images []string
		switch values := t.Input["images"].(type) {
		case []string:
			images = values
		case []any:
			for _, v := range values {
				image, ok := v.(string)
				if !ok {
					return nil, errors.New("one image required")
				}
				images = append(images, image)
			}
		}
		if len(images) != 1 {
			return nil, errors.New("first-frame video requires exactly one image")
		}
		identity, _ := json.Marshal([]any{w.Namespace, t.DispatchID, t.Attempt})
		keySum := sha256.Sum256(identity)
		key := hex.EncodeToString(keySum[:])
		image, err := w.Inputs.Fetch(ctx, key, images[0])
		if err != nil {
			return nil, pending()
		}
		// No signed URL or duplicated transport envelope survives this boundary.
		task := t
		task.Input = nil
		intent := doubao.GenerationIntent{Prompt: prompt, Images: []worker_runtime.Artifact{image}, Model: w.Model, DurationSeconds: w.DurationSeconds, AspectRatio: w.AspectRatio}
		raw, _ := json.Marshal(intent)
		digest := sha256.Sum256(raw)
		cp = &doubao.Checkpoint{Task: task, GenerationKey: key, RequestDigest: hex.EncodeToString(digest[:]), Intent: intent}
		if w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, pending()
		}
	}
	if cp.Reference == "" {
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		lookup, err := w.Generator.Lookup(ctx, cp.GenerationKey)
		if err != nil || lookup.State == doubao.Uncertain {
			return nil, pending()
		}
		switch lookup.State {
		case doubao.Found:
			cp.Reference = string(lookup.Reference)
		case doubao.Absent:
			for _, image := range cp.Intent.Images {
				if w.Files == nil || w.Files.Verify(ctx, image) != nil {
					return nil, pending()
				}
			}
			request := doubao.Request{Key: cp.GenerationKey, Kind: doubao.Video, Prompt: cp.Intent.Prompt, Model: cp.Intent.Model, DurationSeconds: cp.Intent.DurationSeconds, AspectRatio: cp.Intent.AspectRatio}
			for _, image := range cp.Intent.Images {
				request.Images = append(request.Images, doubao.LocalImage{Path: image.LocalPath})
			}
			ref, err := w.Generator.Submit(ctx, request)
			if err != nil {
				return nil, pending()
			}
			cp.Reference = string(ref)
		default:
			return nil, pending()
		}
		if cp.Reference == "" || w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, pending()
		}
	}
	if len(cp.Artifacts) == 0 {
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		observation, err := w.Generator.Observe(ctx, doubao.Reference(cp.Reference))
		if err != nil {
			return nil, pending()
		}
		switch observation.State {
		case doubao.Failed:
			return nil, errors.New("doubao generation failed")
		case doubao.Succeeded:
			if len(observation.Artifacts) != 1 || observation.Artifacts[0].Kind != doubao.Video {
				return nil, pending()
			}
		default:
			return nil, pending()
		}
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		files, err := w.Generator.FetchArtifacts(ctx, doubao.Reference(cp.Reference), observation.Artifacts)
		if err != nil || len(files) != 1 || files[0].ArtifactID != observation.Artifacts[0].ID || files[0].LocalPath == "" {
			return nil, pending()
		}
		cp.Artifacts = []worker_runtime.Artifact{{ID: files[0].ArtifactID, LocalPath: files[0].LocalPath, ChecksumSHA256: files[0].ChecksumSHA256, SizeBytes: files[0].SizeBytes}}
		if w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, pending()
		}
	}
	for i := range cp.Artifacts {
		if cp.Artifacts[i].ObjectPath != "" {
			continue
		}
		if err := w.authorize(ctx, t); err != nil {
			return nil, err
		}
		if w.Files == nil || w.Files.Verify(ctx, cp.Artifacts[i]) != nil {
			return nil, pending()
		}
		object, err := w.Uploader.UploadFile(ctx, w.WorkerID, t, i, cp.Artifacts[i].LocalPath)
		if err != nil || object == "" {
			return nil, pending()
		}
		cp.Artifacts[i].ObjectPath = object
		if w.Checkpoints.Save(ctx, *cp) != nil {
			return nil, pending()
		}
	}
	candidates := make([]map[string]any, 0, len(cp.Artifacts))
	for i, a := range cp.Artifacts {
		candidates = append(candidates, map[string]any{"index": i, "object_path": a.ObjectPath, "is_primary": i == 0})
	}
	return map[string]any{"candidates": candidates, "provenance": map[string]any{"provider": "doubao", "profile": "doubao_studio", "mock": false, "generation_key": cp.GenerationKey}}, nil
}
func (w *Workflow) PendingTasks(ctx context.Context) ([]worker_runtime.Task, error) {
	cps, err := w.Checkpoints.Pending(ctx)
	if err != nil {
		return nil, err
	}
	var tasks []worker_runtime.Task
	for _, cp := range cps {
		tasks = append(tasks, cp.Task)
	}
	return tasks, nil
}
func (w *Workflow) FinishTask(ctx context.Context, t worker_runtime.Task) error {
	cp, err := w.Checkpoints.Load(ctx, t.DispatchID, t.Attempt)
	if err != nil {
		return err
	}
	if cp == nil {
		return nil
	}
	cp.Closed = true
	if err := w.Checkpoints.Save(ctx, *cp); err != nil {
		return fmt.Errorf("close generation checkpoint: %w", err)
	}
	return nil
}

var _ worker_runtime.Executor = (*Workflow)(nil)
var _ worker_runtime.ExecutionRecovery = (*Workflow)(nil)
