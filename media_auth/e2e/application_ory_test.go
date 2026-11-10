//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"gopkg.in/yaml.v3"
	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/authn/internaljwt"
	handler "media_agent/media_auth/biz/handler/application"
	"media_agent/media_auth/biz/middleware"
	router "media_agent/media_auth/biz/router/application"
	tokenservice "media_agent/media_auth/biz/service/token"
	"media_agent/media_auth/biz/shared/session"
)

// Tests the checked-in Oathkeeper rules using a real Oathkeeper process, real
// media_auth handlers and PostgreSQL. Kratos is a controlled session/Admin API
// fixture; this does not claim to exercise Kratos login or Keto projection.
func TestApplicationOryBoundary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Oathkeeper fixture uses Linux host networking")
	}
	kratos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/admin/identities/") {
			role := ""
			if strings.HasSuffix(r.URL.Path, "kratos-admin") {
				role = "platform_admin"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata_admin": map[string]string{"role": role}})
			return
		}
		cookie, e := r.Cookie("ory_kratos_session")
		if e != nil || (cookie.Value != "admin" && cookie.Value != "member") {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"invalid session"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"identity": map[string]any{"id": "kratos-" + cookie.Value, "traits": map[string]any{"email": cookie.Value + "@ory-test.invalid", "name": map[string]string{"first": cookie.Value}}}})
	}))
	defer kratos.Close()
	runApplicationOryBoundary(t, kratos.URL, kratos.URL, map[string]string{"admin": "admin", "member": "member"})
}
func runApplicationOryBoundary(t *testing.T, kratosURL, kratosAdminURL string, cookies map[string]string) {
	f := newApplicationFixture(t)
	apiPort, proxyPort := freeApplicationPort(t), freeApplicationPort(t)
	apiURL := fmt.Sprintf("http://127.0.0.1:%d", apiPort)
	signer, e := session.NewSigner(&configpb.AuthConfig{JwtSecret: "test-only-legacy-session"})
	if e != nil {
		t.Fatal(e)
	}
	verifier := internaljwt.NewVerifier(apiURL+"/.well-known/jwks.json", "http://oathkeeper:4456/", "internal-api", nil)
	oldS, oldU := handler.Service, handler.Users
	handler.Service = f.apps
	handler.Users = f.users
	t.Cleanup(func() { handler.Service = oldS; handler.Users = oldU })
	h := server.New()
	h.Use(middleware.ApplicationProtocol(middleware.SessionWithInternalJWT(signer, tokenservice.NewService(f.userRepo), f.userRepo, verifier, kratosURL, kratosAdminURL)))
	router.Register(h)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		headers := []ut.Header{}
		for k, values := range r.Header {
			for _, v := range values {
				headers = append(headers, ut.Header{Key: k, Value: v})
			}
		}
		response := ut.PerformRequest(h.Engine, r.Method, r.URL.RequestURI(), &ut.Body{Body: strings.NewReader(string(body)), Len: len(body)}, headers...).Result()
		response.Header.VisitAll(func(k, v []byte) { w.Header().Set(string(k), string(v)) })
		w.WriteHeader(response.StatusCode())
		_, _ = w.Write(response.Body())
	}))
	defer upstream.Close()
	dir := t.TempDir()
	cfgBytes, e := os.ReadFile("../../docker-compose/auth/oathkeeper/config.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var cfg map[string]any
	if e = yaml.Unmarshal(cfgBytes, &cfg); e != nil {
		t.Fatal(e)
	}
	serve := cfg["serve"].(map[string]any)
	serve["api"] = map[string]any{"host": "127.0.0.1", "port": apiPort}
	serve["proxy"] = map[string]any{"host": "127.0.0.1", "port": proxyPort}
	cfg["authenticators"].(map[string]any)["cookie_session"].(map[string]any)["config"].(map[string]any)["check_session_url"] = kratosURL + "/sessions/whoami"
	cfg["log"] = map[string]any{"level": "error", "format": "json"}
	cfgBytes, e = yaml.Marshal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	rules, e := os.ReadFile("../../docker-compose/auth/oathkeeper/rules.yaml")
	if e != nil {
		t.Fatal(e)
	}
	rules = []byte(strings.ReplaceAll(string(rules), "http://media_auth:8001", upstream.URL))
	if e = os.WriteFile(filepath.Join(dir, "config.yaml"), cfgBytes, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "rules.yaml"), rules, 0600); e != nil {
		t.Fatal(e)
	}
	writeApplicationJWK(t, filepath.Join(dir, "id_token.jwks.json"))
	name := fmt.Sprintf("media-application-ory-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "run", "--detach", "--rm", "--user", "0:0", "--network", "host", "--name", name, "-v", dir+":/etc/oathkeeper:ro", "ghcr.io/hotttao/ory-oathkeeper:v26.2.0", "serve", "--config", "/etc/oathkeeper/config.yaml", "--sqa-opt-out")
	if e = cmd.Run(); e != nil {
		t.Fatal("Oathkeeper fixture failed to start")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
	})
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ready := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(apiURL + "/health/alive")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Oathkeeper did not become ready")
	}
	request := func(method, path, body, authorization, cookie string, want int) map[string]any {
		t.Helper()
		req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", proxyPort, path), strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "mini-001.local:18080"
		req.Header.Set("Content-Type", "application/json")
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: "ory_kratos_session", Value: cookies[cookie]})
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("edge request failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("edge %s %s status=%d want=%d", method, path, resp.StatusCode, want)
		}
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	request("POST", "/api/applications", `{"id":"ory-app","name":"ORY"}`, "", "member", 403)
	registered := request("POST", "/api/applications", `{"id":"ory-app","name":"ORY"}`, "", "admin", 201)
	secret := registered["client_secret"].(string)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("ory-app:"+secret))
	request("PUT", "/apps/v1/users/17", `{"display_name":"Alice"}`, "", "admin", 401)
	request("PUT", "/apps/v1/users/17", `{"display_name":"Alice"}`, basic, "", 201)
	tok := request("POST", "/apps/v1/users/17/token", "", basic, "", 200)["access_token"].(string)
	request("GET", "/apps/v1/me", "", "Bearer "+tok, "", 200)
	request("GET", "/apps/v1/me", "", basic, "admin", 401)
	request("GET", "/apps/v1/me", "", "Bearer invalid", "admin", 401)
	request("PATCH", "/api/applications/ory-app/status", `{"status":"disabled"}`, "", "admin", 200)
	request("GET", "/apps/v1/me", "", "Bearer "+tok, "admin", 401)
	request("GET", "/apps/v1/not-a-route", "", "", "", 404)
	request("GET", "/internal/token/session", "", "", "", 404)
}
func freeApplicationPort(t *testing.T) int {
	t.Helper()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	return port
}
func writeApplicationJWK(t *testing.T, path string) {
	t.Helper()
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	key.Precompute()
	enc := func(n *big.Int) string { return base64.RawURLEncoding.EncodeToString(n.Bytes()) }
	jwk := map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "application-test", "n": enc(key.N), "e": enc(big.NewInt(int64(key.E))), "d": enc(key.D), "p": enc(key.Primes[0]), "q": enc(key.Primes[1]), "dp": enc(key.Precomputed.Dp), "dq": enc(key.Precomputed.Dq), "qi": enc(key.Precomputed.Qinv)}
	data, e := json.Marshal(map[string]any{"keys": []any{jwk}})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
}
