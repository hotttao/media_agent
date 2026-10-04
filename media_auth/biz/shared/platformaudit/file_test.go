package platformaudit

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testRecord() Record {
	return Record{ID: "event", UserID: "owner", CredentialID: "credential", Operation: "issued", CallerService: "media_auth", Purpose: "login-bootstrap", OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}
func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("audit file unreadable")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var records []Record
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var r struct {
			Event string `json:"event"`
			Record
		}
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			continue
		}
		if r.Event != "platform_token_audit" {
			t.Fatal("unexpected schema")
		}
		records = append(records, r.Record)
	}
	if scanner.Err() != nil {
		t.Fatal("audit scan failed")
	}
	return records
}
func TestFileSinkReopenDuplicatesAndPartialTailFraming(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r := testRecord()
	if s.Publish(t.Context(), r) != nil || s.Close() != nil {
		t.Fatal("durable publication failed")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("audit file not private")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal("reopen failed")
	}
	defer s.Close()
	if s.Publish(t.Context(), r) != nil {
		t.Fatal("stable event replay failed")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("tail fixture failed")
	}
	if _, err := f.WriteString(`{"event":"incomplete`); err != nil {
		t.Fatal("tail fixture write failed")
	}
	if f.Close() != nil {
		t.Fatal("tail fixture close failed")
	}
	r.ID = "after-tail"
	if s.Publish(t.Context(), r) != nil {
		t.Fatal("publication after partial tail failed")
	}
	records := readRecords(t, path)
	if len(records) != 3 || records[0].ID != "event" || records[1].ID != "event" || records[2].ID != "after-tail" {
		t.Fatal("event identity changed or partial tail hid a successful publication")
	}
}
func TestFileSinkRejectsUnsafePathsRecordsAndReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	if _, err := Open("relative-audit-path"); err != ErrSink {
		t.Fatal("relative path accepted")
	}
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal("unsafe file fixture failed")
	}
	if _, err := Open(path); err != ErrSink {
		t.Fatal("public file accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal("mode fixture failed")
	}
	link := filepath.Join(dir, "link")
	if os.Symlink(path, link) != nil {
		t.Fatal("symlink fixture failed")
	}
	if _, err := Open(link); err != ErrSink {
		t.Fatal("symlink accepted")
	}
	if _, err := Open(dir); err != ErrSink {
		t.Fatal("directory accepted")
	}
	publicDir := filepath.Join(dir, "public")
	if os.Mkdir(publicDir, 0700) != nil || os.Chmod(publicDir, 0777) != nil {
		t.Fatal("directory fixture failed")
	}
	if _, err := Open(filepath.Join(publicDir, "audit")); err != ErrSink {
		t.Fatal("writable parent accepted")
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal("safe sink failed")
	}
	defer s.Close()
	for _, mutate := range []func(*Record){func(r *Record) { r.UserID = "header\ninjection" }, func(r *Record) { r.CredentialID = "tos_" + strings.Repeat("a", 64) }, func(r *Record) { r.Operation = "secret" }, func(r *Record) { r.Purpose = "arbitrary" }} {
		r := testRecord()
		mutate(&r)
		if s.Publish(t.Context(), r) != ErrSink {
			t.Fatal("unsafe record published")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if s.Publish(ctx, testRecord()) != ErrSink {
		t.Fatal("cancelled publication accepted")
	}
	if os.Rename(path, path+".old") != nil || os.WriteFile(path, nil, 0600) != nil {
		t.Fatal("replacement fixture failed")
	}
	if s.Publish(t.Context(), testRecord()) != ErrSink {
		t.Fatal("renamed/replaced sink silently confirmed")
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatal("replacement target was modified")
	}
}

type failedAuditFile struct {
	short             bool
	writeErr, syncErr error
	synced            bool
	cancel            context.CancelFunc
}

func (f *failedAuditFile) Write(b []byte) (int, error) {
	if f.short {
		return len(b) - 1, nil
	}
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(b), nil
}
func (f *failedAuditFile) Sync() error {
	f.synced = true
	if f.cancel != nil {
		f.cancel()
	}
	return f.syncErr
}
func (f *failedAuditFile) Close() error { return nil }
func TestFileSinkDoesNotConfirmShortWriteSyncFailureOrCancellation(t *testing.T) {
	for _, f := range []*failedAuditFile{{short: true}, {writeErr: errors.New("private write diagnostic")}, {syncErr: errors.New("private sync diagnostic")}} {
		s := &FileSink{file: f}
		if s.Publish(t.Context(), testRecord()) != ErrSink {
			t.Fatal("failed durable write confirmed")
		}
		if f.short && f.synced {
			t.Fatal("short write treated as complete")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := &FileSink{file: &failedAuditFile{cancel: cancel}}
	if s.Publish(ctx, testRecord()) != ErrSink {
		t.Fatal("cancelled sync confirmed")
	}
	if s.Close() != nil || s.Close() != nil || s.Publish(t.Context(), testRecord()) != ErrSink {
		t.Fatal("closed sink accepted publication")
	}
}
func TestFileSinkConcurrentRecordsRemainReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := testRecord()
			r.ID = fmt.Sprintf("event-%d", i)
			results <- s.Publish(t.Context(), r)
		}(i)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent file publication failed")
		}
	}
	if len(readRecords(t, path)) != 10 {
		t.Fatal("concurrent writes corrupted audit lines")
	}
}
