// Package platformaudit writes fixed credential-free audit records. It has no
// dependency on credential repositories, Domain models or encryption keys.
package platformaudit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrSink = errors.New("platform audit sink unavailable")

type Record struct {
	ID            string    `json:"event_id"`
	UserID        string    `json:"user_id"`
	CredentialID  string    `json:"credential_id"`
	Operation     string    `json:"operation"`
	CallerService string    `json:"caller_service"`
	Purpose       string    `json:"purpose"`
	OccurredAt    time.Time `json:"occurred_at"`
}

func (r Record) valid() bool {
	if !identifier(r.ID) || !identifier(r.UserID) || !identifier(r.CredentialID) || r.OccurredAt.IsZero() {
		return false
	}
	if r.Purpose == "offline-task" && (r.CallerService == "media_agent_gateway" || r.CallerService == "media_control_schedule_worker") {
		return r.Operation == "issued" || r.Operation == "acquired" || r.Operation == "introspected" || r.Operation == "rotated"
	}
	return r.CallerService == "media_auth" && (r.Purpose == "login-bootstrap" && r.Operation == "issued" || r.Purpose == "user-revocation" && r.Operation == "revoked")
}
func identifier(s string) bool {
	if len(s) == 0 || len(s) > 128 || strings.HasPrefix(s, "tos_") {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

type durableFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}
type FileSink struct {
	mu    sync.Mutex
	file  durableFile
	check func() bool
}

func Open(path string) (*FileSink, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrSink
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0022 != 0 {
		return nil, ErrSink
	}
	before, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, ErrSink
	}
	if err == nil && (!before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0) {
		return nil, ErrSink
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, ErrSink
	}
	info, err := f.Stat()
	name, statErr := os.Lstat(path)
	if err != nil || statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !name.Mode().IsRegular() || !os.SameFile(info, name) || (before != nil && !os.SameFile(before, info)) {
		_ = f.Close()
		return nil, ErrSink
	}
	// Persist the directory entry as well as subsequent record contents.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		_ = f.Close()
		return nil, ErrSink
	}
	err = dir.Sync()
	closeErr := dir.Close()
	if err != nil || closeErr != nil {
		_ = f.Close()
		return nil, ErrSink
	}
	check := func() bool {
		info, err := f.Stat()
		name, nameErr := os.Lstat(path)
		parent, parentErr := os.Lstat(filepath.Dir(path))
		return err == nil && nameErr == nil && parentErr == nil && info.Mode().IsRegular() && name.Mode().IsRegular() && info.Mode().Perm()&0077 == 0 && os.SameFile(info, name) && parent.IsDir() && parent.Mode().Perm()&0022 == 0
	}
	return &FileSink{file: f, check: check}, nil
}

func (s *FileSink) Publish(ctx context.Context, r Record) error {
	if s == nil || !r.valid() || ctx.Err() != nil {
		return ErrSink
	}
	r.OccurredAt = r.OccurredAt.UTC()
	payload, err := json.Marshal(struct {
		Event string `json:"event"`
		Record
	}{Event: "platform_token_audit", Record: r})
	if err != nil {
		return ErrSink
	}
	// A leading separator isolates any partial previous write. Readers skip
	// blank lines and quarantine malformed lines, rather than losing a later
	// successfully written event behind a corrupt suffix.
	line := make([]byte, 0, len(payload)+2)
	line = append(line, '\n')
	line = append(line, payload...)
	line = append(line, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil || ctx.Err() != nil || s.check != nil && !s.check() {
		return ErrSink
	}
	n, err := s.file.Write(line)
	if err != nil || n != len(line) {
		return ErrSink
	}
	if err := s.file.Sync(); err != nil {
		return ErrSink
	}
	if ctx.Err() != nil || s.check != nil && !s.check() {
		return ErrSink
	}
	return nil
}
func (s *FileSink) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	if err != nil {
		return ErrSink
	}
	return nil
}
