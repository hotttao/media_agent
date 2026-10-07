package objectstorage

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func uploadInput(key string, payload []byte) UploadInput {
	sha, md := sha256.Sum256(payload), md5.Sum(payload)
	return UploadInput{ObjectPath: key, ContentType: "image/png", SizeBytes: int64(len(payload)), Checksum: hex.EncodeToString(sha[:]), ContentMD5: base64.StdEncoding.EncodeToString(md[:])}
}

func TestPresignedPutBindsChecksumAndConditionalWrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeS3Error(w, http.StatusNotFound, "NoSuchKey") }))
	defer srv.Close()
	c, err := New(context.Background(), Config{Provider: "s3", Endpoint: srv.URL, PublicEndpoint: srv.URL, Region: "us-east-1", Bucket: "media", AccessKey: "fixture", SecretKey: "fixture", ForcePathStyle: true, PresignTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	input := uploadInput("materials/fixture/output", []byte("fixture image bytes"))
	grant, err := c.PresignPut(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(grant.URL)
	if err != nil {
		t.Fatal("invalid presigned address")
	}
	if grant.AlreadyUploaded || grant.ObjectPath != input.ObjectPath || grant.ExpiresAt <= time.Now().Unix() ||
		grant.Headers["If-None-Match"] != "*" || grant.Headers["Content-Md5"] != input.ContentMD5 || !strings.Contains(u.Query().Get("X-Amz-SignedHeaders"), "if-none-match") {
		t.Fatal("upload grant did not bind conditional write and payload checksum")
	}
}

// Explicit running provider contract gate, not a mocked S3 signature check.
func TestS3PresignedPutCompatibility(t *testing.T) {
	if os.Getenv("MEDIA_STORAGE_E2E") != "1" {
		t.Skip("set MEDIA_STORAGE_E2E=1")
	}
	endpoint := envOrDefault("MEDIA_STORAGE_ENDPOINT", "http://127.0.0.1:8333")
	c, err := New(context.Background(), Config{Provider: "s3", Endpoint: endpoint, PublicEndpoint: endpoint, Region: envOrDefault("MEDIA_STORAGE_REGION", "us-east-1"), Bucket: envOrDefault("MEDIA_STORAGE_BUCKET", "media"), AccessKey: envOrDefault("MEDIA_STORAGE_ACCESS_KEY", "media-materials"), SecretKey: envOrDefault("MEDIA_STORAGE_SECRET_KEY", "media-materials-local-secret"), ForcePathStyle: true, PresignTTL: time.Minute})
	if err != nil {
		t.Fatal("provider configuration invalid")
	}
	ctx := context.Background()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	key := "materials/e2e/presigned-put/" + hex.EncodeToString(nonce[:])
	payload := []byte("known worker artifact bytes")
	input := uploadInput(key, payload)
	t.Cleanup(func() {
		if err := c.Delete(ctx, key); err != nil {
			t.Error("own fixture cleanup failed")
		}
	})
	grant, err := c.PresignPut(ctx, input)
	if err != nil {
		t.Fatal("provider upload grant unavailable")
	}
	put := func(body []byte) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, grant.URL, bytes.NewReader(body))
		if err != nil {
			t.Fatal("cannot construct signed upload")
		}
		for key, value := range grant.Headers {
			req.Header.Set(key, value)
		}
		req.ContentLength = int64(len(body))
		resp, err := (&http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			t.Fatal("provider upload transport failed")
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	tampered := append([]byte(nil), payload...)
	tampered[0] ^= 1
	if status := put(tampered); status < 400 {
		t.Fatalf("provider accepted payload violating signed checksum: status=%d", status)
	}
	if _, err := c.Stat(ctx, key); err != ErrNotFound {
		t.Fatal("rejected payload left an object")
	}
	if status := put(payload); status != 200 {
		t.Fatalf("signed direct upload rejected: status=%d", status)
	}
	if status := put(payload); status != 412 {
		t.Fatalf("provider did not enforce write-once condition: status=%d", status)
	}
	replay, err := c.PresignPut(ctx, input)
	if err != nil || !replay.AlreadyUploaded || replay.URL != "" || replay.ObjectPath != key {
		t.Fatal("HEAD did not confirm committed upload replay")
	}
	changed := input
	changed.Checksum = strings.Repeat("0", 64)
	if _, err := c.PresignPut(ctx, changed); err != ErrConflict {
		t.Fatal("different payload reused an immutable object")
	}
	resolved, err := c.PresignGet(ctx, key)
	if err != nil {
		t.Fatal("uploaded object not readable")
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(resolved.URL)
	if err != nil {
		t.Fatal("direct object download failed")
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != 200 || !bytes.Equal(got, payload) {
		t.Fatal("stored artifact bytes differ")
	}
}
