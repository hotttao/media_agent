package objectstorage

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifiedPresignUsesOneHEADAndRejectsChangedDigest(t *testing.T) {
	var heads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Error("unexpected storage method")
			w.WriteHeader(405)
			return
		}
		heads.Add(1)
		w.Header().Set("Content-Length", "42")
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("X-Amz-Meta-Checksum-Sha256", "recorded-digest")
		w.WriteHeader(200)
	}))
	defer server.Close()
	c, err := New(t.Context(), Config{Provider: "s3", Endpoint: server.URL, PublicEndpoint: server.URL, Region: "us-east-1", Bucket: "media", AccessKey: "access", SecretKey: "secret", ForcePathStyle: true, PresignTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := c.PresignVerifiedGet(t.Context(), "test.png", "recorded-digest")
	if err != nil || resolved.URL == "" || resolved.SizeBytes != 42 || resolved.ChecksumSHA256 != "recorded-digest" || heads.Load() != 1 {
		t.Fatal("verified resolution must use one HEAD")
	}
	for _, checksum := range []string{"changed", ""} {
		resolved, err := c.PresignVerifiedGet(t.Context(), "test.png", checksum)
		if !errors.Is(err, ErrConflict) || resolved.URL != "" {
			t.Fatal("signed an object without a matching digest")
		}
	}
}
