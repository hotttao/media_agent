package agent_product_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"media_agent/hertz_infra/authn/resourceaccess"
	gwDAL "media_agent/media_agent_gateway/biz/dal"
	repo "media_agent/media_agent_gateway/biz/dal/repo/mcp_catalog"
	handler "media_agent/media_agent_gateway/biz/handler/mcp_catalog"
	"media_agent/media_agent_gateway/biz/middleware"
	router "media_agent/media_agent_gateway/biz/router/mcp_catalog"
	service "media_agent/media_agent_gateway/biz/service/mcp_catalog"
	"media_agent/media_agent_gateway/biz/shared/client"
	"media_agent/media_agent_gateway/biz/workflow"
)

type catalogUserAuth struct{}

func (catalogUserAuth) Authenticate(_ context.Context, authorization, _ string) (client.GatewayUser, error) {
	user := strings.TrimPrefix(authorization, "Bearer ")
	if user == "" {
		return client.GatewayUser{}, client.ErrAgentTokenUnauthorized
	}
	role := "user"
	if user == "platform-admin" {
		role = "admin"
	}
	return client.GatewayUser{UserID: user, Role: role}, nil
}
func TestEnterpriseMCPCatalogHTTPAndHistory(t *testing.T) {
	db, _ := databaseWithMigrations(t, "../../../media_agent_gateway/migrations")
	var revoked atomic.Bool
	facts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Media-Auth-Internal-Token") != "facts" {
			w.WriteHeader(401)
			return
		}
		var request map[string]string
		must(t, json.NewDecoder(r.Body).Decode(&request))
		if r.URL.Path == "/internal/teams:byUser" {
			_ = json.NewEncoder(w).Encode(map[string]any{"team_ids": []string{}})
			return
		}
		role, org := "member", "org"
		if request["user_id"] == "org-admin" {
			role = "admin"
		}
		if request["user_id"] == "foreign-admin" {
			role = "admin"
			org = "foreign"
		}
		ids := []string{org}
		if revoked.Load() {
			ids = []string{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"organization_ids": ids, "organizations": []map[string]string{{"organization_id": org, "role": role, "status": "active"}}})
	}))
	defer facts.Close()
	catalog := service.New(repo.New(db), gwDAL.NewTxRunner(db), []string{"mcp.example.test"})
	handler.Configure(catalog, nil)
	handler.ConfigureManagement(&workflow.MCPCatalogManagement{Catalog: catalog, Facts: resourceaccess.New(facts.URL, "facts", nil)})
	middleware.ConfigureAgentTokenManagement(catalogUserAuth{})
	middleware.Configure("", "internal")
	h := server.New()
	router.Register(h)
	api := mount(h)
	defer api.Close()
	request := func(user, method, path string, body any, status int) map[string]any {
		t.Helper()
		content, err := json.Marshal(body)
		must(t, err)
		req, err := http.NewRequest(method, api.URL+path, bytes.NewReader(content))
		must(t, err)
		req.Header.Set("Authorization", "Bearer "+user)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Control-Service-Token", "internal")
		resp, err := http.DefaultClient.Do(req)
		must(t, err)
		defer resp.Body.Close()
		payload, err := io.ReadAll(resp.Body)
		must(t, err)
		if resp.StatusCode != status {
			t.Fatalf("%s %s as %s: status=%d expected=%d body=%s", method, path, user, resp.StatusCode, status, payload)
		}
		var value map[string]any
		must(t, json.Unmarshal(payload, &value))
		return value
	}
	body := map[string]any{"definition_id": "enterprise-search", "organization_id": "org", "service_name": "search", "endpoint": "https://mcp.example.test/mcp", "transport": "streamable_http", "authentication_type": "bearer", "status": "enabled"}
	request("member", "POST", "/api/v1/mcp-definitions", body, 403)
	request("foreign-admin", "POST", "/api/v1/mcp-definitions", body, 403)
	request("platform-admin", "POST", "/api/v1/mcp-definitions", body, 403)
	value := request("org-admin", "POST", "/api/v1/mcp-definitions", body, 201)
	if value["organization_id"] != "org" || value["version"] != float64(1) {
		t.Fatal(value)
	}
	request("member", "GET", "/api/v1/mcp-definitions/enterprise-search", nil, 200)
	request("foreign-admin", "GET", "/api/v1/mcp-definitions/enterprise-search", nil, 403)
	list := request("member", "GET", "/api/v1/mcp-definitions?organization_id=org", nil, 200)
	if len(list["definitions"].([]any)) != 2 {
		t.Fatal(list)
	}
	request("foreign-admin", "GET", "/api/v1/mcp-definitions?organization_id=org", nil, 403)
	list = request("foreign-admin", "GET", "/api/v1/mcp-definitions?organization_id=foreign", nil, 200)
	if len(list["definitions"].([]any)) != 1 {
		t.Fatal(list)
	}
	body["expected_version"] = 1
	body["organization_id"] = "foreign"
	request("foreign-admin", "PUT", "/api/v1/mcp-definitions/enterprise-search", body, 403)
	body["organization_id"] = "org"
	body["authentication_type"] = "session_context"
	request("org-admin", "PUT", "/api/v1/mcp-definitions/enterprise-search", body, 400)
	body["authentication_type"] = "api_key"
	request("org-admin", "PUT", "/api/v1/mcp-definitions/enterprise-search", body, 200)
	request("org-admin", "PUT", "/api/v1/mcp-definitions/enterprise-search", body, 409)
	history := request("member", "GET", "/api/v1/mcp-definitions/enterprise-search?version=1", nil, 200)
	if history["authentication_type"] != "bearer" {
		t.Fatal("public fixed definition read returned current authentication", history)
	}
	request("foreign-admin", "GET", "/api/v1/mcp-definitions/enterprise-search?version=1", nil, 403)
	request("member", "GET", "/api/v1/mcp-definitions/enterprise-search?version=-1", nil, 400)
	first, err := catalog.Version(context.Background(), "enterprise-search", 1)
	must(t, err)
	if first.AuthenticationType != "bearer" || first.OrganizationID != "org" {
		t.Fatal(first)
	}
	body["expected_version"] = 2
	body["status"] = "retired"
	request("org-admin", "PUT", "/api/v1/mcp-definitions/enterprise-search", body, 200)
	body["expected_version"] = 3
	body["status"] = "enabled"
	request("org-admin", "PUT", "/api/v1/mcp-definitions/enterprise-search", body, 409)
	revoked.Store(true)
	request("org-admin", "GET", "/api/v1/mcp-definitions/enterprise-search", nil, 403)
	request("org-admin", "PUT", "/api/v1/mcp-definitions/enterprise-search", body, 403)
	revoked.Store(false)
	var versions, events int64
	must(t, db.Table("media_agent_gateway.mcp_definition_versions").Where("id=?", "enterprise-search").Count(&versions).Error)
	must(t, db.Table("media_agent_gateway.mcp_catalog_events").Where("definition_id=?", "enterprise-search").Count(&events).Error)
	if versions != 3 || events != 3 {
		t.Fatal(fmt.Sprintf("versions=%d events=%d", versions, events))
	}
}
