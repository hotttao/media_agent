// library_object_upload_e2e_test.go verifies the public gateway upload path.
// Services are managed outside the test per harness/contributing/e2e.md.
// Borrow MEDIA_API_TOKEN; no browser session or simulated login is required.
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
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
	organizationID := os.Getenv("MEDIA_E2E_ORGANIZATION_ID")
	if organizationID == "" {
		t.Skip("MEDIA_E2E_ORGANIZATION_ID 未设，跳过网关上传 e2e")
	}
	apiToken := os.Getenv("MEDIA_API_TOKEN")
	if apiToken == "" {
		t.Skip("MEDIA_API_TOKEN 未设，跳过网关上传 e2e")
	}

	pngA := libraryPNGFixture(t, color.NRGBA{R: 220, A: 255})
	pngB := libraryPNGFixture(t, color.NRGBA{B: 220, A: 255})
	idempotencyKey := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	firstStatus, firstBody := uploadLibraryObject(t, gatewayAddr, apiToken, organizationID, idempotencyKey, pngA)
	if firstStatus != http.StatusOK {
		t.Fatalf("first upload: status=%d", firstStatus)
	}
	var first struct {
		ObjectPath string `json:"object_path"`
		Checksum   string `json:"checksum"`
		Size       int64  `json:"size_bytes"`
		Type       string `json:"content_type"`
	}
	if err := json.Unmarshal(firstBody, &first); err != nil || first.ObjectPath == "" || first.Checksum == "" {
		t.Fatal("first upload did not return object metadata")
	}
	digest := sha256.Sum256(pngA)
	if first.Checksum != hex.EncodeToString(digest[:]) || first.Size != int64(len(pngA)) || first.Type != "image/png" {
		t.Fatal("uploaded image metadata does not match its actual content")
	}

	retryStatus, retryBody := uploadLibraryObject(t, gatewayAddr, apiToken, organizationID, idempotencyKey, pngA)
	if retryStatus != http.StatusOK {
		t.Fatalf("idempotent retry: status=%d", retryStatus)
	}
	var retry struct {
		ObjectPath string `json:"object_path"`
		Checksum   string `json:"checksum"`
	}
	if err := json.Unmarshal(retryBody, &retry); err != nil || retry.ObjectPath != first.ObjectPath || retry.Checksum != first.Checksum {
		t.Fatal("retry object identity or checksum differs")
	}

	conflictStatus, _ := uploadLibraryObject(t, gatewayAddr, apiToken, organizationID, idempotencyKey, pngB)
	if conflictStatus != http.StatusConflict {
		t.Fatalf("same idempotency key with different content: status=%d, want 409", conflictStatus)
	}
	resolvePath := "/api/v1/library-objects:resolve?organization_id=" + url.QueryEscape(organizationID) + "&object_path=" + url.QueryEscape(first.ObjectPath)
	status, data := doJSONBearer(t, gatewayAddr, http.MethodGet, resolvePath, apiToken, nil)
	var resolved struct {
		URL string `json:"url"`
	}
	if status != http.StatusOK || json.Unmarshal(data, &resolved) != nil || resolved.URL == "" {
		t.Fatal("uploaded image could not be resolved")
	}
	location, err := url.Parse(resolved.URL)
	if err != nil || location.Host == "" || location.User != nil || (location.Scheme != "http" && location.Scheme != "https") {
		t.Fatal("invalid uploaded image URL")
	}
	// The signed storage URL, not the gateway credential, authorizes this read.
	probe, err := http.NewRequest(http.MethodGet, resolved.URL, nil)
	if err != nil {
		t.Fatal("cannot construct image download")
	}
	storage := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := storage.Do(probe)
	if err != nil {
		t.Fatal("uploaded image download failed")
	}
	downloaded, readErr := io.ReadAll(io.LimitReader(response.Body, int64(len(pngA))+1))
	response.Body.Close()
	if response.StatusCode != http.StatusOK || readErr != nil || !bytes.Equal(downloaded, pngA) {
		t.Fatal("downloaded image differs from upload")
	}
	decoded, err := png.Decode(bytes.NewReader(downloaded))
	if err != nil || decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 2 {
		t.Fatal("uploaded image cannot be decoded at its original dimensions")
	}
	status, _ = doJSONBearer(t, gatewayAddr, http.MethodGet, "/api/v1/library-objects:resolve?organization_id=not-a-member&object_path="+url.QueryEscape(first.ObjectPath), apiToken, nil)
	if status != http.StatusNotFound {
		t.Fatal("foreign Organization resolved an uploaded object")
	}
	status, _ = uploadLibraryObject(t, gatewayAddr, apiToken, "not-a-member", idempotencyKey, pngA)
	if status != http.StatusNotFound {
		t.Fatal("foreign Organization accepted an upload")
	}
}

func libraryPNGFixture(t *testing.T, pixel color.NRGBA) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	canvas.SetNRGBA(0, 0, pixel)
	var data bytes.Buffer
	if err := png.Encode(&data, canvas); err != nil {
		t.Fatal("encode fixture image")
	}
	return data.Bytes()
}

func TestLibraryPNGFixtureIsValidAndDistinct(t *testing.T) {
	red, blue := color.NRGBA{R: 220, A: 255}, color.NRGBA{B: 220, A: 255}
	a, b := libraryPNGFixture(t, red), libraryPNGFixture(t, blue)
	if bytes.Equal(a, b) {
		t.Fatal("different fixture pixels produced identical upload bodies")
	}
	for _, fixture := range []struct {
		data  []byte
		pixel color.NRGBA
	}{{a, red}, {b, blue}} {
		decoded, err := png.Decode(bytes.NewReader(fixture.data))
		if err != nil || decoded.Bounds().Dx() != 2 || decoded.Bounds().Dy() != 2 || color.NRGBAModel.Convert(decoded.At(0, 0)) != fixture.pixel {
			t.Fatal("fixture image dimensions or pixels are invalid")
		}
	}
}

func uploadLibraryObject(t *testing.T, gatewayAddr, apiToken, organizationID, idempotencyKey string, data []byte) (int, []byte) {
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
	req.Header.Set("Authorization", "Bearer "+apiToken)
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
