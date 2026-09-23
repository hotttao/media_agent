package objectstorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestS3Compatibility verifies the subset of S3 semantics required by the
// materials library against a real provider. The test intentionally does not
// start infrastructure; the e2e harness owns the provider lifecycle.
func TestS3Compatibility(t *testing.T) {
	if os.Getenv("MEDIA_STORAGE_E2E") != "1" {
		t.Skip("set MEDIA_STORAGE_E2E=1 to test a running S3-compatible provider")
	}

	endpoint := envOrDefault("MEDIA_STORAGE_ENDPOINT", "http://127.0.0.1:8333")
	client, err := New(context.Background(), Config{
		Provider:       "s3",
		Endpoint:       endpoint,
		PublicEndpoint: endpoint,
		Region:         envOrDefault("MEDIA_STORAGE_REGION", "us-east-1"),
		Bucket:         envOrDefault("MEDIA_STORAGE_BUCKET", "media"),
		AccessKey:      envOrDefault("MEDIA_STORAGE_ACCESS_KEY", "media-materials"),
		SecretKey:      envOrDefault("MEDIA_STORAGE_SECRET_KEY", "media-materials-local-secret"),
		ForcePathStyle: true,
		PresignTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	body := []byte("media-agent-s3-compatibility")
	sum := sha256.Sum256(body)
	objectPath := "materials/e2e/objectstorage/compatibility-" + hex.EncodeToString(sum[:8]) + ".txt"
	t.Cleanup(func() {
		if err := client.Delete(context.Background(), objectPath); err != nil {
			t.Errorf("cleanup Delete: %v", err)
		}
	})

	input := PutInput{ObjectPath: objectPath, ContentType: "text/plain", Checksum: hex.EncodeToString(sum[:]), SizeBytes: int64(len(body))}
	input.Body = bytes.NewReader(body)
	if _, err := client.PutIfAbsent(context.Background(), input); err != nil {
		t.Fatalf("first Put: %v", err)
	}
	input.Body = bytes.NewReader(body)
	if _, err := client.PutIfAbsent(context.Background(), input); err != nil {
		t.Fatalf("idempotent Put: %v", err)
	}
	conflict := input
	conflict.Checksum = "different-checksum"
	conflict.Body = bytes.NewReader([]byte("different"))
	if _, err := client.PutIfAbsent(context.Background(), conflict); err != ErrConflict {
		t.Fatalf("conflicting Put error=%v, want ErrConflict", err)
	}

	resolved, err := client.PresignGet(context.Background(), objectPath)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	response, err := http.Get(resolved.URL) //nolint:gosec // URL is produced by the test's configured provider.
	if err != nil {
		t.Fatalf("GET presigned URL: %v", err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read presigned response: %v", err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Equal(got, body) {
		t.Fatalf("presigned GET status=%d body=%q", response.StatusCode, got)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
