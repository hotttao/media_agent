// Package gpt_checkpoints persists GPT browser executions separately from Studio.
package gpt_checkpoints

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sync"

	"media_agent/media_worker/biz/domain/gpt"
)

var ErrStorage = errors.New("GPT checkpoint unavailable")
var keyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Store struct {
	dir string
	mu  sync.Mutex
}
type imageDTO struct {
	Path string `json:"path"`
	SHA  string `json:"sha256"`
	Size int64  `json:"size"`
}
type artifactDTO struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA    string `json:"sha256"`
	Size   int64  `json:"size"`
	Object string `json:"object_path,omitempty"`
}
type requestDTO struct {
	Key      string     `json:"key"`
	Prompt   string     `json:"prompt"`
	Images   []imageDTO `json:"images"`
	Size     string     `json:"size"`
	WebModel string     `json:"web_model"`
}
type checkpointDTO struct {
	DispatchID        string        `json:"dispatch_id"`
	Attempt           int           `json:"attempt"`
	CapabilityKey     string        `json:"capability_key"`
	CapabilityVersion int           `json:"capability_version"`
	TraceParent       string        `json:"trace_parent,omitempty"`
	Request           requestDTO    `json:"request"`
	Reference         string        `json:"reference,omitempty"`
	Artifacts         []artifactDTO `json:"artifacts,omitempty"`
	Closed            bool          `json:"closed,omitempty"`
}

func Open(namespace string) (*Store, error) {
	dir := filepath.Join(namespace, "gpt-checkpoints")
	if os.MkdirAll(dir, 0700) != nil {
		return nil, ErrStorage
	}
	s, err := os.Lstat(dir)
	if err != nil || !s.IsDir() || s.Mode().Perm()&0077 != 0 {
		return nil, ErrStorage
	}
	return &Store{dir: dir}, nil
}
func filename(id string, attempt int) string {
	raw, _ := json.Marshal([]any{id, attempt})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]) + ".json"
}
func valid(d checkpointDTO) bool {
	return d.DispatchID != "" && d.Attempt > 0 && d.CapabilityVersion == 1 && (d.CapabilityKey == gpt.TextCapability || d.CapabilityKey == gpt.ReferenceCapability) && keyPattern.MatchString(d.Request.Key) && d.Request.Prompt != ""
}
func (s *Store) load(id string, attempt int) (*checkpointDTO, error) {
	path := filepath.Join(s.dir, filename(id, attempt))
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return nil, ErrStorage
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrStorage
	}
	var d checkpointDTO
	if json.Unmarshal(raw, &d) != nil || !valid(d) || d.DispatchID != id || d.Attempt != attempt {
		return nil, ErrStorage
	}
	return &d, nil
}
func (s *Store) Load(ctx context.Context, id string, attempt int) (*gpt.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	d, err := s.load(id, attempt)
	if err != nil || d == nil {
		return nil, err
	}
	c := fromDTO(*d)
	return &c, nil
}
func (s *Store) Save(ctx context.Context, c gpt.Checkpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	d := toDTO(c)
	if !valid(d) {
		return ErrStorage
	}
	old, err := s.load(d.DispatchID, d.Attempt)
	if err != nil {
		return err
	}
	if old != nil {
		if !reflect.DeepEqual(old.Request, d.Request) || old.CapabilityKey != d.CapabilityKey || old.CapabilityVersion != d.CapabilityVersion || old.TraceParent != d.TraceParent || (old.Closed && !d.Closed) || (old.Reference != "" && old.Reference != d.Reference) || len(d.Artifacts) < len(old.Artifacts) {
			return ErrStorage
		}
		for i, a := range old.Artifacts {
			b := d.Artifacts[i]
			if a.ID != b.ID || a.Path != b.Path || a.SHA != b.SHA || a.Size != b.Size || (a.Object != "" && a.Object != b.Object) {
				return ErrStorage
			}
		}
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return ErrStorage
	}
	f, err := os.CreateTemp(s.dir, ".gpt-")
	if err != nil {
		return ErrStorage
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrStorage
	}
	if os.Rename(name, filepath.Join(s.dir, filename(d.DispatchID, d.Attempt))) != nil {
		return ErrStorage
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return ErrStorage
	}
	defer dir.Close()
	return dir.Sync()
}
func (s *Store) Pending(ctx context.Context) ([]gpt.Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, ErrStorage
	}
	var out []gpt.Checkpoint
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.dir, e.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
			return nil, ErrStorage
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, ErrStorage
		}
		var d checkpointDTO
		if json.Unmarshal(raw, &d) != nil || !valid(d) || filename(d.DispatchID, d.Attempt) != e.Name() {
			return nil, ErrStorage
		}
		if !d.Closed {
			out = append(out, fromDTO(d))
		}
	}
	return out, nil
}
func toDTO(c gpt.Checkpoint) checkpointDTO {
	r := requestDTO{Key: c.Request.Key, Prompt: c.Request.Prompt, Size: c.Request.Size, WebModel: c.Request.WebModel}
	for _, i := range c.Request.Images {
		r.Images = append(r.Images, imageDTO{i.Path, i.ChecksumSHA256, i.SizeBytes})
	}
	d := checkpointDTO{DispatchID: c.DispatchID, Attempt: c.Attempt, CapabilityKey: c.CapabilityKey, CapabilityVersion: c.CapabilityVersion, TraceParent: c.TraceParent, Request: r, Reference: string(c.Reference), Closed: c.Closed}
	for _, a := range c.Artifacts {
		d.Artifacts = append(d.Artifacts, artifactDTO{a.ArtifactID, a.LocalPath, a.ChecksumSHA256, a.SizeBytes, a.ObjectPath})
	}
	return d
}
func fromDTO(d checkpointDTO) gpt.Checkpoint {
	r := gpt.Request{Key: d.Request.Key, Prompt: d.Request.Prompt, Size: d.Request.Size, WebModel: d.Request.WebModel}
	for _, i := range d.Request.Images {
		r.Images = append(r.Images, gpt.LocalImage{Path: i.Path, ChecksumSHA256: i.SHA, SizeBytes: i.Size})
	}
	c := gpt.Checkpoint{DispatchID: d.DispatchID, Attempt: d.Attempt, CapabilityKey: d.CapabilityKey, CapabilityVersion: d.CapabilityVersion, TraceParent: d.TraceParent, Request: r, Reference: gpt.Reference(d.Reference), Closed: d.Closed}
	for _, a := range d.Artifacts {
		c.Artifacts = append(c.Artifacts, gpt.UploadedArtifact{Download: gpt.Download{ArtifactID: a.ID, LocalPath: a.Path, ChecksumSHA256: a.SHA, SizeBytes: a.Size}, ObjectPath: a.Object})
	}
	return c
}

var _ gpt.Repository = (*Store)(nil)
