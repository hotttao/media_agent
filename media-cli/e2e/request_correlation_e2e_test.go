package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// Read-only HTTP gate: no log collector, Worker or Job is started by the test.
func TestRequestCorrelationGateway(t *testing.T) {
	addr, token := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN")
	org := os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	if addr == "" || token == "" || org == "" {
		t.Skip("set MEDIA_GATEWAY_ADDR, MEDIA_API_TOKEN and MEDIA_E2E_ORGANIZATION_ID")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	traceID := hex.EncodeToString(id[:])
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, path := range []string{"/api/v1/workflows?page_size=1", "/api/v1/materials?page_size=1&organization_id=" + url.QueryEscape(org)} {
		for _, withParent := range []bool{true, false} {
			req, err := http.NewRequest("GET", addr+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			if withParent {
				req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("%s status=%d", path, resp.StatusCode)
			}
			got := resp.Header.Get("X-Trace-ID")
			decoded, err := hex.DecodeString(got)
			if err != nil || len(decoded) != 16 || got == "00000000000000000000000000000000" {
				t.Fatalf("%s missing valid response trace ID", path)
			}
			if withParent && got != traceID {
				t.Fatalf("%s did not preserve upstream trace ID", path)
			}
			if !withParent && got == traceID {
				t.Fatalf("%s reused an unrelated request trace ID", path)
			}
		}
	}
}
