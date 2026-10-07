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

	"media_agent/media_worker/internal/gateway"
)

type Record struct {
	Task        gateway.Task   `json:"task"`
	Key         string         `json:"key"`
	Output      map[string]any `json:"output,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	FailureKind string         `json:"failure_kind,omitempty"`
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
func (s *Store) path(r Record) string {
	return filepath.Join(s.dir, hash(r.Task.DispatchID+"\x00"+r.Key)+".json")
}
func valid(r Record) bool {
	return r.Task.DispatchID != "" && r.Task.Attempt > 0 && r.Key != "" && (r.Reason != "" || len(r.Output) > 0)
}

func (s *Store) Save(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(r) {
		return ErrStorage
	}
	// Inputs are not needed for result replay; do not copy frozen prompts into
	// this journal. The exact dispatch and attempt identify the original Task.
	r.Task.Input, r.Task.InputJSON = nil, ""
	data, err := json.Marshal(r)
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

func (s *Store) Pending() ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, ErrStorage
	}
	var records []Record
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<20 {
			return nil, ErrStorage
		}
		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		var r Record
		if err != nil || json.Unmarshal(data, &r) != nil || !valid(r) || filepath.Base(s.path(r)) != entry.Name() {
			return nil, ErrStorage
		}
		records = append(records, r)
	}
	return records, nil
}

// Only the store's own pending file is removed after native acknowledgement.
// Rejected/expired results are retained separately for inspection, not resent.
func (s *Store) Finish(r Record, rejected bool) error {
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
