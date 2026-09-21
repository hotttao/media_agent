// library_object_upload_e2e_test.go verifies the public gateway upload path.
// Services are managed outside the test per harness/contributing/e2e.md.
// MEDIA_E2E_KRATOS_SESSION_COOKIE is the value of an authenticated
// ory_kratos_session cookie obtained through the normal login UI.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestLibraryObjectUploadE2E(t *testing.T) {
	gatewayAddr := os.Getenv("MEDIA_GATEWAY_ADDR")
	if gatewayAddr == "" {
		t.Skip("MEDIA_GATEWAY_ADDR 未设，跳过网关上传 e2e")
	}
	organizationID := os.Getenv("MEDIA_MATERIALS_E2E_ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("MEDIA_MATERIALS_E2E_ORGANIZATION_ID 未设，跳过网关上传 e2e")
	}
	kratosSession := os.Getenv("MEDIA_E2E_KRATOS_SESSION_COOKIE")
	if kratosSession == "" {
		t.Skip("MEDIA_E2E_KRATOS_SESSION_COOKIE 未设，跳过需要 Ory 会话的网关上传 e2e")
	}

	pngA := []byte("\x89PNG\r\n\x1a\ne2e-image-a")
	pngB := []byte("\x89PNG\r\n\x1a\ne2e-image-b")
	idempotencyKey := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	firstStatus, firstBody := uploadLibraryObject(t, gatewayAddr, kratosSession, organizationID, idempotencyKey, pngA)
	if firstStatus != http.StatusOK {
		t.Fatalf("first upload: status=%d body=%s", firstStatus, firstBody)
	}
	var first struct {
		ObjectPath string `json:"object_path"`
		Checksum   string `json:"checksum"`
	}
	if err := json.Unmarshal(firstBody, &first); err != nil || first.ObjectPath == "" || first.Checksum == "" {
		t.Fatalf("decode first upload: err=%v body=%s", err, firstBody)
	}

	retryStatus, retryBody := uploadLibraryObject(t, gatewayAddr, kratosSession, organizationID, idempotencyKey, pngA)
	if retryStatus != http.StatusOK {
		t.Fatalf("idempotent retry: status=%d body=%s", retryStatus, retryBody)
	}
	var retry struct {
		ObjectPath string `json:"object_path"`
		Checksum   string `json:"checksum"`
	}
	if err := json.Unmarshal(retryBody, &retry); err != nil || retry.ObjectPath != first.ObjectPath || retry.Checksum != first.Checksum {
		t.Fatalf("retry result differs: err=%v first=%+v body=%s", err, first, retryBody)
	}

	conflictStatus, _ := uploadLibraryObject(t, gatewayAddr, kratosSession, organizationID, idempotencyKey, pngB)
	if conflictStatus != http.StatusConflict {
		t.Fatalf("same idempotency key with different content: status=%d, want 409", conflictStatus)
	}
}

func uploadLibraryObject(t *testing.T, gatewayAddr, kratosSession, organizationID, idempotencyKey string, data []byte) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "image.png")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	endpoint := gatewayAddr + "/api/v1/library-objects:upload?organization_id=" + url.QueryEscape(organizationID) +
		"&scope=ai_clothing&purpose=material"
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		t.Fatalf("new upload request: %v", err)
	}
	req.Header.Set("Cookie", "ory_kratos_session="+kratosSession)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, responseBody
}
