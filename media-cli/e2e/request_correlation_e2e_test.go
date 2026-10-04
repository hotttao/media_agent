package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Read-only HTTP gate: no log collector, Worker or Job is started by the test.
func TestRequestCorrelationGateway(t *testing.T) {
	checkRequestCorrelation(t, "")
}

// The log gate requires actual mounted application logs. No collectors or
// asynchronous jobs are started, and no credential or full log is printed.
func TestRequestCorrelationLogsGateway(t *testing.T) {
	root := os.Getenv("MEDIA_E2E_REQUEST_LOG_DIR")
	if root == "" {
		t.Skip("configure MEDIA_E2E_REQUEST_LOG_DIR for mounted application logs")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("request log root must be absolute")
	}
	checkRequestCorrelation(t, root)
}

func checkRequestCorrelation(t *testing.T, root string) {
	t.Helper()
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
	for _, target := range []struct{ path, service string }{
		{"/api/v1/workflows?page_size=1", "media-workflow"},
		{"/api/v1/materials?page_size=1&organization_id=" + url.QueryEscape(org), "media-materials"},
	} {
		path := target.path
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
			if root != "" {
				awaitRequestTraceLog(t, filepath.Join(root, target.service, "app.log"), got)
			}
		}
	}
}

func awaitRequestTraceLog(t *testing.T, path, traceID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		// Reuse the bounded mounted-log reader; it does not emit file contents.
		lines, err := agentAuditLogTail(path)
		if err != nil {
			t.Fatal("cannot read mounted request log")
		}
		for _, line := range lines {
			var entry struct {
				Trace string `json:"trace_id"`
				Span  string `json:"span_id"`
				Flags string `json:"trace_flags"`
			}
			if json.Unmarshal(line, &entry) != nil || entry.Trace != traceID {
				continue
			}
			span, err := hex.DecodeString(entry.Span)
			if err != nil || len(span) != 8 || entry.Span == "0000000000000000" || entry.Span == "00f067aa0ba902b7" || entry.Flags != "00" {
				t.Fatal("request log lost its independent unsampled server span")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("response trace ID not found in mounted request log")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
