package e2e

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Services are provisioned externally. This gate never starts a service,
// imports a keyring/DAL, logs a response body or persists a returned Secret.
// Rotation applies only to a newly registered dedicated test user.
func TestPlatformTokenMTLSRuntime(t *testing.T) {
	addr, authAddr, materialDir, auditPath := os.Getenv("MEDIA_PLATFORM_TOKEN_ADDR"), os.Getenv("MEDIA_AUTH_PRIVATE_ADDR"), os.Getenv("MEDIA_PLATFORM_TEST_MATERIAL_DIR"), os.Getenv("MEDIA_PLATFORM_AUDIT_FILE")
	if addr == "" || authAddr == "" || materialDir == "" || auditPath == "" {
		t.Skip("configure private Auth/mTLS addresses, test client materials and mounted audit file")
	}
	if !strings.HasPrefix(addr, "https://") || !filepath.IsAbs(materialDir) || !filepath.IsAbs(auditPath) {
		t.Fatal("invalid internal test configuration")
	}
	if _, err := os.ReadFile(auditPath); err != nil {
		t.Fatal("private audit file unavailable; run the test as an authorized audit reader")
	}
	caPEM, err := os.ReadFile(filepath.Join(materialDir, "client-ca.pem"))
	if err != nil {
		t.Fatal("test CA unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("test CA invalid")
	}
	newClient := func(name string) *http.Client {
		t.Helper()
		config := &tls.Config{RootCAs: roots, ServerName: "media_auth", MinVersion: tls.VersionTLS13}
		if name != "" {
			cert, err := tls.LoadX509KeyPair(filepath.Join(materialDir, name+".pem"), filepath.Join(materialDir, name+"-key.pem"))
			if err != nil {
				t.Fatal("test client materials unavailable")
			}
			config.Certificates = []tls.Certificate{cert}
		}
		transport := &http.Transport{TLSClientConfig: config}
		t.Cleanup(transport.CloseIdleConnections)
		return &http.Client{Transport: transport, Timeout: 10 * time.Second}
	}
	gateway, schedule, noCert := newClient("gateway"), newClient("schedule-worker"), newClient("")
	request := func(client *http.Client, url string, payload any, want int) []byte {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal("test input invalid")
		}
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			t.Fatal("test request invalid")
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("internal test request failed")
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
		if err != nil || resp.StatusCode != want {
			t.Fatalf("internal endpoint status=%d want=%d", resp.StatusCode, want)
		}
		if strings.HasPrefix(url, addr+"/internal/platform-token/") && resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("credential response can be cached")
		}
		return body
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("test randomness unavailable")
	}
	username := fmt.Sprintf("platform-runtime-%d", time.Now().UnixNano())
	var registered struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	plainTransport := &http.Transport{}
	defer plainTransport.CloseIdleConnections()
	plain := &http.Client{Transport: plainTransport, Timeout: 10 * time.Second}
	if err := json.Unmarshal(request(plain, authAddr+"/api/auth/register", map[string]any{"username": username, "password": hex.EncodeToString(random[:]), "display_name": "Platform runtime test"}, 201), &registered); err != nil || registered.User.ID == "" {
		t.Fatal("dedicated test user unavailable")
	}
	userID := registered.User.ID
	acquire := func(client *http.Client) string {
		t.Helper()
		var response struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(request(client, addr+"/internal/platform-token/acquire", map[string]string{"user_id": userID, "purpose": "offline-task"}, 200), &response); err != nil || len(response.Token) != 68 {
			t.Fatal("credential not acquired")
		}
		return response.Token
	}
	first := acquire(gateway)
	if second := acquire(schedule); second != first {
		t.Fatal("two callers received different current credentials")
	}
	introspect := func(secret string, active bool) string {
		t.Helper()
		var response struct {
			Active bool   `json:"active"`
			User   string `json:"user_id"`
			ID     string `json:"credential_id"`
		}
		if err := json.Unmarshal(request(gateway, addr+"/internal/platform-token/introspect", map[string]string{"token": secret, "purpose": "offline-task"}, 200), &response); err != nil || response.Active != active {
			t.Fatal("wrong credential activity")
		}
		if active && (response.User != userID || response.ID == "") {
			t.Fatal("wrong principal ownership")
		}
		if !active && (response.User != "" || response.ID != "") {
			t.Fatal("inactive response leaked ownership")
		}
		return response.ID
	}
	oldID := introspect(first, true)
	request(gateway, addr+"/internal/platform-token/acquire", map[string]string{"user_id": userID, "purpose": "login-bootstrap"}, 403)
	request(gateway, addr+"/internal/platform-token/acquire", map[string]string{"user_id": "unknown-test-user", "purpose": "offline-task"}, 403)
	var rotated struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(request(schedule, addr+"/internal/platform-token/rotate", map[string]any{"user_id": userID, "purpose": "offline-task", "expires_at": time.Now().Add(time.Hour).Unix()}, 200), &rotated); err != nil || rotated.Token == "" || rotated.Token == first {
		t.Fatal("dedicated credential not rotated")
	}
	introspect(first, false)
	newID := introspect(rotated.Token, true)
	if newID == oldID {
		t.Fatal("rotation reused old credential identity")
	}
	introspect("invalid-platform-token", false)
	// Missing certificate and forged forwarding metadata must never enter service.
	req, _ := http.NewRequest(http.MethodPost, addr+"/internal/platform-token/acquire", strings.NewReader(`{"purpose":"offline-task"}`))
	req.Header.Set("X-Caller-Service", "media_agent_gateway")
	if resp, err := noCert.Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("missing certificate admitted")
	}
	// The public listener's internal-auth guard rejects before the mTLS guard.
	request(plain, authAddr+"/internal/platform-token/acquire", map[string]string{"user_id": userID, "purpose": "offline-task"}, 401)
	// Wait for the real background Outbox publisher's private durable file.
	deadline := time.Now().Add(15 * time.Second)
	for {
		contents, err := os.ReadFile(auditPath)
		if err == nil {
			if bytes.Contains(contents, []byte(first)) || bytes.Contains(contents, []byte(rotated.Token)) {
				t.Fatal("audit included credential material")
			}
			issued, rotation, acquired, inspected := false, false, false, false
			for _, line := range bytes.Split(contents, []byte("\n")) {
				if len(bytes.TrimSpace(line)) == 0 {
					continue
				}
				var event map[string]json.RawMessage
				if json.Unmarshal(line, &event) != nil {
					t.Fatal("invalid durable audit record")
				}
				if len(event) != 8 || event["token"] != nil || event["secret"] != nil || event["ciphertext"] != nil || event["token_hash"] != nil {
					t.Fatal("audit schema violated")
				}
				var owner, operation, credential string
				_ = json.Unmarshal(event["user_id"], &owner)
				_ = json.Unmarshal(event["operation"], &operation)
				_ = json.Unmarshal(event["credential_id"], &credential)
				if owner != userID {
					continue
				}
				issued = issued || operation == "issued" && credential == oldID
				rotation = rotation || operation == "rotated" && credential == newID
				acquired = acquired || operation == "acquired" && credential == oldID
				inspected = inspected || operation == "introspected"
			}
			if issued && rotation && acquired && inspected {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("real audit delivery did not converge")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
