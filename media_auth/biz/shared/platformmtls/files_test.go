package platformmtls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCertificateFileBoundary(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "private.pem")
	if err := os.WriteFile(private, []byte("fixture only"), 0600); err != nil {
		t.Fatal("fixture file failed")
	}
	if _, err := readFile(private, true); err != nil {
		t.Fatal("private regular file rejected")
	}
	public := filepath.Join(dir, "public.pem")
	if err := os.WriteFile(public, []byte("fixture only"), 0644); err != nil {
		t.Fatal("fixture file failed")
	}
	if _, err := readFile(public, false); err != nil {
		t.Fatal("public read-only certificate rejected")
	}
	if _, err := readFile(public, true); err == nil {
		t.Fatal("publicly readable private key accepted")
	}
	link := filepath.Join(dir, "link.pem")
	if err := os.Symlink(private, link); err != nil {
		t.Fatal("fixture symlink failed")
	}
	empty, oversized := filepath.Join(dir, "empty.pem"), filepath.Join(dir, "oversized.pem")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal("fixture file failed")
	}
	if err := os.WriteFile(oversized, make([]byte, 65537), 0600); err != nil {
		t.Fatal("fixture file failed")
	}
	for _, path := range []string{"relative.pem", link, empty, oversized, dir, filepath.Join(dir, "missing.pem")} {
		if _, err := readFile(path, true); err == nil {
			t.Fatal("unsafe file accepted")
		}
	}
	if err := os.Chmod(public, 0666); err != nil {
		t.Fatal("fixture permissions failed")
	}
	if _, err := readFile(public, false); err == nil {
		t.Fatal("writable certificate accepted")
	}
	if _, _, err := LoadFiles(private, private, private, map[string]string{}); err == nil {
		t.Fatal("malformed certificate material accepted")
	}
}
