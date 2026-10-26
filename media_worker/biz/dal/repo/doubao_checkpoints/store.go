// Package doubao_checkpoints persists generation facts separately from delivery results.
package doubao_checkpoints

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"media_agent/media_worker/biz/domain/doubao"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

var ErrStorage = errors.New("worker checkpoint storage unavailable")

type Store struct {
	dir string
	mu  sync.Mutex
}
type taskDTO struct {
	DispatchID        string `json:"dispatch_id"`
	Attempt           int    `json:"attempt"`
	CapabilityKey     string `json:"capability_key"`
	CapabilityVersion int    `json:"capability_version"`
	TraceParent       string `json:"trace_parent,omitempty"`
}
type artifactDTO struct {
	ChecksumSHA256 string `json:"checksum_sha256"`
	SizeBytes      int64  `json:"size_bytes"`
	ID             string `json:"id"`
	LocalPath      string `json:"local_path"`
	ObjectPath     string `json:"object_path,omitempty"`
}
type intentDTO struct {
	Prompt          string        `json:"prompt"`
	Images          []artifactDTO `json:"images"`
	Model           string        `json:"model"`
	DurationSeconds int           `json:"duration_seconds"`
	AspectRatio     string        `json:"aspect_ratio"`
}
type checkpointDTO struct {
	Task          taskDTO       `json:"task"`
	Intent        intentDTO     `json:"intent"`
	GenerationKey string        `json:"generation_key"`
	RequestDigest string        `json:"request_digest"`
	Reference     string        `json:"reference,omitempty"`
	Artifacts     []artifactDTO `json:"artifacts,omitempty"`
	Closed        bool          `json:"closed,omitempty"`
}

// The owning results Store holds the process lock for this private namespace.
func Open(namespace string) (*Store, error) {
	dir := filepath.Join(namespace, "checkpoints")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, ErrStorage
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrStorage
	}
	return &Store{dir: dir}, nil
}
func hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (s *Store) path(dispatch string, attempt int) string {
	data, _ := json.Marshal([]any{dispatch, attempt})
	return filepath.Join(s.dir, hash(string(data))+".json")
}
func (s *Store) Save(ctx context.Context, c doubao.Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if c.Task.DispatchID == "" || c.Task.Attempt < 1 || c.GenerationKey == "" || c.RequestDigest == "" {
		return ErrStorage
	}
	data, err := json.Marshal(toDisk(c))
	if err != nil {
		return ErrStorage
	}
	target := s.path(c.Task.DispatchID, c.Task.Attempt)
	if old, err := s.load(c.Task.DispatchID, c.Task.Attempt); err != nil {
		return err
	} else if old != nil {
		if !reflect.DeepEqual(old.Intent, c.Intent) || old.Task.DispatchID != c.Task.DispatchID || old.Task.Attempt != c.Task.Attempt || old.GenerationKey != c.GenerationKey || old.RequestDigest != c.RequestDigest || old.Closed && !c.Closed || old.Reference != "" && c.Reference != old.Reference || len(c.Artifacts) < len(old.Artifacts) {
			return ErrStorage
		}
	}
	if old, err := s.load(c.Task.DispatchID, c.Task.Attempt); err != nil {
		return err
	} else if old != nil {
		for idx, a := range old.Artifacts {
			b := c.Artifacts[idx]
			if a.ID != b.ID || a.LocalPath != b.LocalPath || a.ChecksumSHA256 != b.ChecksumSHA256 || a.SizeBytes != b.SizeBytes || a.ObjectPath != "" && a.ObjectPath != b.ObjectPath {
				return ErrStorage
			}
		}
	}
	file, err := os.CreateTemp(s.dir, ".write-")
	if err != nil {
		return ErrStorage
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return ErrStorage
	}
	if err = os.Rename(name, target); err != nil {
		return ErrStorage
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return ErrStorage
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return ErrStorage
	}
	return nil
}
func (s *Store) load(dispatch string, attempt int) (*doubao.Checkpoint, error) {
	filename := s.path(dispatch, attempt)
	info, err := os.Lstat(filename)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return nil, ErrStorage
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, ErrStorage
	}
	var dto checkpointDTO
	if json.Unmarshal(data, &dto) != nil || dto.Task.DispatchID != dispatch || dto.Task.Attempt != attempt || dto.GenerationKey == "" || dto.RequestDigest == "" {
		return nil, ErrStorage
	}
	c := fromDisk(dto)
	return &c, nil
}
func (s *Store) Load(ctx context.Context, dispatch string, attempt int) (*doubao.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return s.load(dispatch, attempt)
}
func (s *Store) Pending(ctx context.Context) ([]doubao.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, ErrStorage
	}
	var out []doubao.Checkpoint
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
			return nil, ErrStorage
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		var dto checkpointDTO
		if err != nil || json.Unmarshal(data, &dto) != nil {
			return nil, ErrStorage
		}
		c, err := s.load(dto.Task.DispatchID, dto.Task.Attempt)
		if err != nil || c == nil || filepath.Base(s.path(dto.Task.DispatchID, dto.Task.Attempt)) != entry.Name() {
			return nil, ErrStorage
		}
		if !c.Closed {
			out = append(out, *c)
		}
	}
	return out, nil
}
func artifactsToDisk(as []worker_runtime.Artifact) []artifactDTO {
	var out []artifactDTO
	for _, a := range as {
		out = append(out, artifactDTO{ID: a.ID, LocalPath: a.LocalPath, ObjectPath: a.ObjectPath, ChecksumSHA256: a.ChecksumSHA256, SizeBytes: a.SizeBytes})
	}
	return out
}
func artifactsFromDisk(as []artifactDTO) []worker_runtime.Artifact {
	var out []worker_runtime.Artifact
	for _, a := range as {
		out = append(out, worker_runtime.Artifact{ID: a.ID, LocalPath: a.LocalPath, ObjectPath: a.ObjectPath, ChecksumSHA256: a.ChecksumSHA256, SizeBytes: a.SizeBytes})
	}
	return out
}
func toDisk(c doubao.Checkpoint) checkpointDTO {
	t := c.Task
	i := c.Intent
	return checkpointDTO{Task: taskDTO{t.DispatchID, t.Attempt, t.CapabilityKey, t.CapabilityVersion, t.TraceParent}, Intent: intentDTO{i.Prompt, artifactsToDisk(i.Images), i.Model, i.DurationSeconds, i.AspectRatio}, GenerationKey: c.GenerationKey, RequestDigest: c.RequestDigest, Reference: c.Reference, Artifacts: artifactsToDisk(c.Artifacts), Closed: c.Closed}
}
func fromDisk(c checkpointDTO) doubao.Checkpoint {
	t := c.Task
	i := c.Intent
	return doubao.Checkpoint{Task: worker_runtime.Task{DispatchID: t.DispatchID, Attempt: t.Attempt, CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion, TraceParent: t.TraceParent}, Intent: doubao.GenerationIntent{Prompt: i.Prompt, Images: artifactsFromDisk(i.Images), Model: i.Model, DurationSeconds: i.DurationSeconds, AspectRatio: i.AspectRatio}, GenerationKey: c.GenerationKey, RequestDigest: c.RequestDigest, Reference: c.Reference, Artifacts: artifactsFromDisk(c.Artifacts), Closed: c.Closed}
}

var _ doubao.CheckpointRepository = (*Store)(nil)
