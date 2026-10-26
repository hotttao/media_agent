// Package results stores only completed function outputs awaiting native Task
// acknowledgement. It is not a scheduler or a second workflow state machine.
package results

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"media_agent/media_worker/biz/domain/execution"
)

type recordDTO struct {
	Task        taskDTO        `json:"task"`
	Key         string         `json:"key"`
	Output      map[string]any `json:"output,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	FailureKind string         `json:"failure_kind,omitempty"`
}

type taskDTO struct {
	DispatchID        string `json:"dispatch_id"`
	Attempt           int    `json:"attempt"`
	CapabilityKey     string `json:"capability_key"`
	CapabilityVersion int    `json:"capability_version"`
	InputJSON         string `json:"input_json"`
	LeaseExpiresAt    int64  `json:"lease_expires_at"`
	TraceParent       string `json:"trace_parent,omitempty"`
}

type Store struct {
	dir  string
	lock *os.File
	mu   sync.Mutex
}

var ErrStorage = errors.New("worker result storage unavailable; inspect private state directory")

func hash(value string) string { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }

// The credential is hashed into the namespace, never stored in a record.
// Exclusive process ownership prevents concurrent replay by two clients.
func Open(base, endpoint, worker, credential string) (*Store, error) {
	if base == "" || endpoint == "" || worker == "" || credential == "" {
		return nil, ErrStorage
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, ErrStorage
	}
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrStorage
	}
	dir := filepath.Join(base, hash(strings.TrimRight(endpoint, "/")+"\x00"+worker+"\x00"+credential))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, ErrStorage
	}
	info, err = os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrStorage
	}
	lockPath := filepath.Join(dir, ".lock")
	if info, err := os.Lstat(lockPath); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, ErrStorage
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, ErrStorage
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, ErrStorage
	}
	return &Store{dir: dir, lock: lock}, nil
}

func (s *Store) Close() error { return s.lock.Close() }
func (s *Store) path(r execution.Result) string {
	return filepath.Join(s.dir, hash(r.Task.DispatchID+"\x00"+r.Key)+".json")
}
func valid(r execution.Result) bool {
	return r.Task.DispatchID != "" && r.Task.Attempt > 0 && r.Key != "" && (r.Reason != "" || len(r.Output) > 0)
}

func (s *Store) Save(r execution.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(r) {
		return ErrStorage
	}
	// Inputs are not needed for result replay; do not copy frozen prompts into
	// this journal. The exact dispatch and attempt identify the original Task.
	r.Task.Input = nil
	data, err := json.Marshal(toDisk(r))
	if err != nil {
		return ErrStorage
	}
	if info, err := os.Lstat(s.path(r)); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return ErrStorage
	}
	if old, err := os.ReadFile(s.path(r)); err == nil {
		if string(old) != string(data) {
			return ErrStorage
		}
		return nil
	} else if !os.IsNotExist(err) {
		return ErrStorage
	}
	file, err := os.CreateTemp(s.dir, ".write-")
	if err != nil {
		return ErrStorage
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return ErrStorage
	}
	if err := os.Rename(name, s.path(r)); err != nil {
		return ErrStorage
	}
	return s.syncDir()
}

func (s *Store) Pending() ([]execution.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, ErrStorage
	}
	var records []execution.Result
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<20 {
			return nil, ErrStorage
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		var disk recordDTO
		if err != nil || json.Unmarshal(data, &disk) != nil {
			return nil, ErrStorage
		}
		r := fromDisk(disk)
		if !valid(r) || filepath.Base(s.path(r)) != entry.Name() {
			return nil, ErrStorage
		}
		records = append(records, r)
	}
	return records, nil
}

// Only the store's own pending file is removed after native acknowledgement.
// Rejected/expired results are retained separately for inspection, not resent.
func (s *Store) Finish(r execution.Result, rejected bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(r) {
		return ErrStorage
	}
	var err error
	if rejected {
		err = os.Rename(s.path(r), s.path(r)+".rejected")
	} else {
		err = os.Remove(s.path(r))
	}
	if err != nil && !os.IsNotExist(err) {
		return ErrStorage
	}
	return s.syncDir()
}

func (s *Store) syncDir() error {
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

func toDisk(r execution.Result) recordDTO {
	t := r.Task
	return recordDTO{Task: taskDTO{DispatchID: t.DispatchID, Attempt: t.Attempt, CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion, LeaseExpiresAt: t.LeaseExpiresAt, TraceParent: t.TraceParent}, Key: r.Key, Output: r.Output, Reason: r.Reason, FailureKind: r.FailureKind}
}
func fromDisk(d recordDTO) execution.Result {
	t := d.Task
	return execution.Result{Task: execution.Task{DispatchID: t.DispatchID, Attempt: t.Attempt, CapabilityKey: t.CapabilityKey, CapabilityVersion: t.CapabilityVersion, LeaseExpiresAt: t.LeaseExpiresAt, TraceParent: t.TraceParent}, Key: d.Key, Output: d.Output, Reason: d.Reason, FailureKind: d.FailureKind}
}

var _ execution.ResultRepository = (*Store)(nil)

// Namespace is the credential-isolated directory protected by this Store's lock.
func (s *Store) Namespace() string { return s.dir }
