package media_input

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBoundedPrivateInputAndChangedFileDetection(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("gateway identity leaked to input download")
		}
		_, _ = w.Write(png)
	}))
	defer server.Close()
	d := Downloader{Root: filepath.Join(t.TempDir(), "private")}
	a, err := d.Fetch(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", server.URL+"?private-signed-url")
	if err != nil || d.Verify(context.Background(), a) != nil {
		t.Fatal(a, err)
	}
	if os.WriteFile(a.LocalPath, []byte("changed"), 0600) != nil {
		t.Fatal("change file")
	}
	if d.Verify(context.Background(), a) == nil {
		t.Fatal("changed input accepted")
	}
}
func TestUnsupportedInputMediaIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>not an image</html>")) }))
	defer server.Close()
	if _, err := (Downloader{Root: filepath.Join(t.TempDir(), "private")}).Fetch(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", server.URL); err == nil {
		t.Fatal("HTML accepted as reference image")
	}
}
