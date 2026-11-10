//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"media_agent/media_auth/e2e/fixtures"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestApplicationRealKratosOathkeeperLogin(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("isolated Ory containers require host networking")
	}
	db := fixtures.NewDatabase(t)
	dir := t.TempDir()
	publicPort, adminPort := freeApplicationPort(t), freeApplicationPort(t)
	public := fmt.Sprintf("http://127.0.0.1:%d", publicPort)
	admin := fmt.Sprintf("http://127.0.0.1:%d", adminPort)
	raw, e := os.ReadFile("../../docker-compose/auth/kratos/kratos.yml")
	if e != nil {
		t.Fatal(e)
	}
	var cfg map[string]any
	if e = yaml.Unmarshal(raw, &cfg); e != nil {
		t.Fatal(e)
	}
	cfg["dsn"] = db.DSN
	serve := cfg["serve"].(map[string]any)
	serve["public"].(map[string]any)["port"] = publicPort
	serve["public"].(map[string]any)["base_url"] = public + "/"
	serve["admin"].(map[string]any)["port"] = adminPort
	serve["admin"].(map[string]any)["base_url"] = admin + "/"
	self := cfg["selfservice"].(map[string]any)
	self["default_browser_return_url"] = public + "/"
	self["allowed_return_urls"] = []string{public + "/"}
	raw, e = yaml.Marshal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "kratos.yml"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	schema, e := os.ReadFile("../../docker-compose/auth/kratos/identity.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "identity.schema.json"), schema, 0600); e != nil {
		t.Fatal(e)
	}
	image := "ghcr.io/hotttao/ory-kratos:v26.2.0"
	docker := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		out, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if e != nil {
			t.Fatalf("isolated Kratos command: %v: %s", e, out)
		}
	}
	docker("run", "--rm", "--user", "0:0", "--network", "host", "-v", dir+":/etc/config/kratos:ro", image, "migrate", "sql", "--read-from-env", "--yes", "--config", "/etc/config/kratos/kratos.yml")
	name := fmt.Sprintf("media-application-kratos-%d", time.Now().UnixNano())
	docker("run", "--detach", "--rm", "--user", "0:0", "--network", "host", "--name", name, "-v", dir+":/etc/config/kratos:ro", image, "serve", "--dev", "--config", "/etc/config/kratos/kratos.yml")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
	})
	client := &http.Client{Timeout: 3 * time.Second}
	ready := false
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); {
		r, e := client.Get(admin + "/health/ready")
		if e == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Kratos did not become ready")
	}
	cookies := map[string]string{}
	for _, role := range []string{"admin", "member"} {
		identity := map[string]any{"schema_id": "default", "traits": map[string]string{"email": role + "@application-test.invalid"}, "credentials": map[string]any{"password": map[string]any{"config": map[string]string{"password": "Fixture-Only-Password-9047!"}}}}
		if role == "admin" {
			identity["metadata_admin"] = map[string]string{"role": "platform_admin"}
		}
		raw, _ := json.Marshal(identity)
		r, e := client.Post(admin+"/admin/identities", "application/json", strings.NewReader(string(raw)))
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != 201 {
			t.Fatalf("Kratos create identity: %d", r.StatusCode)
		}
		jar, _ := cookiejar.New(nil)
		browser := &http.Client{Jar: jar, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, _ := http.NewRequest("GET", public+"/self-service/login/browser", nil)
		req.Header.Set("Accept", "application/json")
		r, e = browser.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		var flow struct {
			UI struct {
				Action string `json:"action"`
				Nodes  []struct {
					Attributes struct {
						Name  string `json:"name"`
						Value any    `json:"value"`
					} `json:"attributes"`
				} `json:"nodes"`
			} `json:"ui"`
		}
		e = json.NewDecoder(r.Body).Decode(&flow)
		r.Body.Close()
		if e != nil || r.StatusCode != 200 {
			t.Fatalf("Kratos browser flow: %d %v", r.StatusCode, e)
		}
		var csrf string
		for _, node := range flow.UI.Nodes {
			if node.Attributes.Name == "csrf_token" {
				csrf, _ = node.Attributes.Value.(string)
			}
		}
		if csrf == "" {
			t.Fatal("Kratos CSRF missing")
		}
		raw, _ = json.Marshal(map[string]string{"method": "password", "identifier": role + "@application-test.invalid", "password": "Fixture-Only-Password-9047!", "csrf_token": csrf})
		req, _ = http.NewRequest("POST", flow.UI.Action, strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		r, e = browser.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != 200 && r.StatusCode != 303 {
			t.Fatalf("Kratos browser login: %d", r.StatusCode)
		}
		u, _ := url.Parse(public)
		for _, cookie := range jar.Cookies(u) {
			if cookie.Name == "ory_kratos_session" {
				cookies[role] = cookie.Value
			}
		}
		if cookies[role] == "" {
			t.Fatal("Kratos login did not establish cookie")
		}
	}
	runApplicationOryBoundary(t, public, admin, cookies)
}
