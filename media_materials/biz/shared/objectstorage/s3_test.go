package objectstorage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

type testObject struct {
	body        []byte
	contentType string
	checksum    string
}

func TestS3ClientPutIsIdempotentAndPresigns(t *testing.T) {
	objects := map[string]testObject{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectPath := strings.TrimPrefix(r.URL.Path, "/media/")
		switch r.Method {
		case http.MethodHead:
			object, ok := objects[objectPath]
			if !ok {
				writeS3Error(w, http.StatusNotFound, "NoSuchKey")
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(object.body)))
			w.Header().Set("Content-Type", object.contentType)
			w.Header().Set("ETag", `"test-etag"`)
			w.Header().Set("X-Amz-Meta-Checksum-Sha256", object.checksum)
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			if r.Header.Get("If-None-Match") != "*" {
				t.Errorf("If-None-Match=%q, want *", r.Header.Get("If-None-Match"))
			}
			if _, exists := objects[objectPath]; exists {
				writeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed")
				return
			}
			body, _ := io.ReadAll(r.Body)
			objects[objectPath] = testObject{body: body, contentType: r.Header.Get("Content-Type"),
				checksum: r.Header.Get("X-Amz-Meta-Checksum-Sha256")}
			w.Header().Set("ETag", `"test-etag"`)
			w.WriteHeader(http.StatusOK)
		default:
			writeS3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed")
		}
	}))
	defer server.Close()

	client, err := New(context.Background(), Config{Provider: "s3", Endpoint: server.URL, PublicEndpoint: server.URL,
		Region: "us-east-1", Bucket: "media", AccessKey: "access", SecretKey: "secret", ForcePathStyle: true, PresignTTL: time.Minute})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	input := PutInput{ObjectPath: "materials/org/file.png", ContentType: "image/png", Checksum: "checksum-a",
		SizeBytes: 3, Body: strings.NewReader("png")}
	first, err := client.PutIfAbsent(context.Background(), input)
	if err != nil || first.Checksum != "checksum-a" {
		t.Fatalf("first Put=%+v, %v", first, err)
	}
	input.Body = strings.NewReader("png")
	second, err := client.PutIfAbsent(context.Background(), input)
	if err != nil || second.Checksum != "checksum-a" {
		t.Fatalf("idempotent Put=%+v, %v", second, err)
	}
	conflict := input
	conflict.Checksum = "checksum-b"
	conflict.Body = strings.NewReader("new")
	if _, err := client.PutIfAbsent(context.Background(), conflict); err != ErrConflict {
		t.Fatalf("conflicting Put error=%v, want ErrConflict", err)
	}

	resolved, err := client.PresignGet(context.Background(), input.ObjectPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	parsed, err := url.Parse(resolved.URL)
	if err != nil || parsed.Host != strings.TrimPrefix(server.URL, "http://") ||
		parsed.Path != "/media/materials/org/file.png" || parsed.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("unexpected presigned URL: %q (%v)", resolved.URL, err)
	}
	if resolved.ExpiresAt <= time.Now().Unix() {
		t.Fatalf("expires_at=%d is not in the future", resolved.ExpiresAt)
	}
}

func TestS3ClientRejectsIncompleteConfiguration(t *testing.T) {
	if _, err := New(context.Background(), Config{Provider: "s3"}); err == nil {
		t.Fatal("incomplete configuration must fail")
	}
}

func writeS3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<Error><Code>%s</Code><Message>test</Message></Error>", code)
}
