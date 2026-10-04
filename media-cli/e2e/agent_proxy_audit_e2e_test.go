package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The application services are already running. This gate reads their actual
// mounted JSON logs; it never starts collectors, Runtime or generation Jobs.
func TestAgentProxyAuditGateway(t *testing.T) {
	addr, user, workspace, root := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_WORKSPACE_ID"), os.Getenv("MEDIA_E2E_AGENT_AUDIT_LOG_DIR")
	if addr == "" || user == "" || workspace == "" || root == "" {
		t.Skip("configure gateway, User API Token, Workspace and mounted application log root")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("audit log root must be absolute")
	}
	request := func(method, path, token string, body any, want int) []byte {
		t.Helper()
		status, result := doJSONBearer(t, addr, method, path, token, body)
		if status != want {
			t.Fatalf("audit gate response status=%d want=%d", status, want)
		}
		return result
	}
	var facts struct {
		Config string `json:"agent_configuration_id"`
		Owner  string `json:"created_by"`
		Org    string `json:"organization_id"`
	}
	if err := json.Unmarshal(request("GET", "/api/v1/workspaces/"+workspace, user, nil, 200), &facts); err != nil || facts.Config == "" || facts.Owner == "" || facts.Org == "" {
		t.Fatal("test Workspace identity unavailable")
	}
	var issued struct {
		Secret string `json:"secret"`
		Record struct {
			ID string `json:"id"`
		} `json:"api_token"`
	}
	command := map[string]any{"name": "Agent audit gate", "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "idempotency_key": fmt.Sprintf("audit-http-%d", time.Now().UnixNano())}
	if err := json.Unmarshal(request("POST", "/api/v1/agent-clients/"+facts.Config+"/api-tokens", user, command, 201), &issued); err != nil || issued.Secret == "" || issued.Record.ID == "" {
		t.Fatal("test credential unavailable")
	}
	t.Cleanup(func() { request("POST", "/api/api-tokens/"+issued.Record.ID+"/revoke", user, nil, 200) })
	for _, attempt := range []struct {
		service string
		status  int
	}{{"workflow", 200}, {"materials", 200}, {"materials", 404}} {
		service := attempt.service
		uri := "/api/v1/jobs?page_size=1&probe=query-must-not-log"
		if service == "materials" {
			uri = "/api/v1/materials?page_size=1&organization_id=" + facts.Org + "&probe=query-must-not-log"
		}
		if attempt.status == 404 {
			uri = "/api/v1/materials?organization_id=not-a-member&probe=query-must-not-log"
		}
		var envelope struct {
			Status int    `json:"status_code"`
			Trace  string `json:"trace_id"`
		}
		if err := json.Unmarshal(request("POST", "/api/agent/v1/requests", issued.Secret, map[string]any{"service": service, "method": "GET", "uri": uri}, 200), &envelope); err != nil || envelope.Status != attempt.status || len(envelope.Trace) != 32 {
			t.Fatal("audit request not successful")
		}
		for _, application := range []string{"media-agent-gateway", "media-" + service} {
			path := filepath.Join(root, application, "app.log")
			deadline := time.Now().Add(5 * time.Second)
			found := false
			for time.Now().Before(deadline) {
				lines, err := agentAuditLogTail(path)
				if err != nil {
					t.Fatal("cannot read mounted application audit log")
				}
				for _, line := range lines {
					var wrapper struct {
						Message string `json:"msg"`
						Trace   string `json:"trace_id"`
					}
					if json.Unmarshal(line, &wrapper) != nil {
						continue
					}
					var e struct {
						Event      string `json:"event"`
						Trace      string `json:"trace_id"`
						User       string `json:"user_id"`
						Actor      string `json:"actor_id"`
						Credential string `json:"credential_id"`
						Org        string `json:"organization_id"`
						Workspace  string `json:"workspace_id"`
						Service    string `json:"service"`
						Route      string `json:"route_template"`
						Outcome    string `json:"outcome"`
						Status     int    `json:"status_code"`
					}
					if json.Unmarshal([]byte(wrapper.Message), &e) != nil || e.Trace != envelope.Trace {
						continue
					}
					wantEvent := "agent_business_request"
					if application == "media-agent-gateway" {
						wantEvent = "agent_proxy_request"
					}
					if e.Event != wantEvent {
						continue
					}
					wantOutcome := "success"
					if attempt.status != 200 {
						wantOutcome = "rejected"
					}
					if wrapper.Trace != envelope.Trace || e.User != facts.Owner || e.Actor != "Agent:"+facts.Config || e.Credential != issued.Record.ID || e.Org != facts.Org || e.Workspace != workspace || e.Service != service || e.Status != attempt.status || e.Outcome != wantOutcome || strings.ContainsAny(e.Route, "?=&") {
						t.Fatal("audit lost identity, result or trace association")
					}
					for _, forbidden := range []string{issued.Secret, "query-must-not-log", "Authorization", "internal-assertion"} {
						if strings.Contains(wrapper.Message, forbidden) {
							t.Fatal("audit leaked credential or query")
						}
					}
					found = true
				}
				if found {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if !found {
				t.Fatalf("no correlated %s audit event", application)
			}
		}
		t.Logf("%s status=%d gateway/business audit trace_id=%s", service, attempt.status, envelope.Trace)
	}
}

func agentAuditLogTail(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	const limit int64 = 2 << 20
	if info.Size() > limit {
		if _, err = f.Seek(-limit, io.SeekEnd); err != nil {
			return nil, err
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, err
	}
	return bytes.Split(data, []byte("\n")), nil
}
