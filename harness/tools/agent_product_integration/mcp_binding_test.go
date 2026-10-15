package agent_product_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	metadataPB "media_agent/hertz_gen/model/media_control/agent_metadata"
	bindingPB "media_agent/hertz_gen/model/media_plugin/mcp_binding"
	"media_agent/hertz_infra/mcpprotocol"
	"media_agent/hertz_infra/metadataartifact"
	"media_agent/hertz_infra/sessioncredential"
	gwDAL "media_agent/media_agent_gateway/biz/dal"
	catalogRepo "media_agent/media_agent_gateway/biz/dal/repo/mcp_catalog"
	catalogDomain "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
	catalogHandler "media_agent/media_agent_gateway/biz/handler/mcp_catalog"
	gwMiddleware "media_agent/media_agent_gateway/biz/middleware"
	catalogRouter "media_agent/media_agent_gateway/biz/router/mcp_catalog"
	catalogService "media_agent/media_agent_gateway/biz/service/mcp_catalog"
	gwClient "media_agent/media_agent_gateway/biz/shared/client"
	gwWorkflow "media_agent/media_agent_gateway/biz/workflow"
	oauthRepo "media_agent/media_auth/biz/dal/repo/mcp_oauth"
	oauthDomain "media_agent/media_auth/biz/domain/mcp_oauth"
	oauthHandler "media_agent/media_auth/biz/handler/mcp_oauth"
	oauthService "media_agent/media_auth/biz/service/mcp_oauth"
	oauthClient "media_agent/media_auth/biz/shared/client/mcp"
	"media_agent/media_control/biz/dal"
	metadataRepo "media_agent/media_control/biz/dal/repo/agent_configuration"
	workspaceRepo "media_agent/media_control/biz/dal/repo/workspace"
	metadataHandler "media_agent/media_control/biz/handler/agent_metadata"
	workspaceHandler "media_agent/media_control/biz/handler/workspace"
	"media_agent/media_control/biz/middleware"
	metadataRouter "media_agent/media_control/biz/router/agent_metadata"
	workspaceRouter "media_agent/media_control/biz/router/workspace"
	metadataService "media_agent/media_control/biz/service/agent_configuration"
	workspaceService "media_agent/media_control/biz/service/workspace"
	controlClient "media_agent/media_control/biz/shared/client"
	authClient "media_agent/media_control/biz/shared/client/mediaauth"
	"media_agent/media_control/biz/shared/secret"
	metadataFlow "media_agent/media_control/biz/workflow/metadata_update"
	pluginDAL "media_agent/media_plugin/biz/dal"
	bindingRepo "media_agent/media_plugin/biz/dal/repo/plugin_binding"
	accessHandler "media_agent/media_plugin/biz/handler/mcp_access"
	bindingHandler "media_agent/media_plugin/biz/handler/mcp_binding"
	pluginMiddleware "media_agent/media_plugin/biz/middleware"
	accessRouter "media_agent/media_plugin/biz/router/mcp_access"
	bindingRouter "media_agent/media_plugin/biz/router/mcp_binding"
	bindingService "media_agent/media_plugin/biz/service/plugin_binding"
	pluginClient "media_agent/media_plugin/biz/shared/client"
	pluginSecret "media_agent/media_plugin/biz/shared/secret"
	pluginWorkflow "media_agent/media_plugin/biz/workflow"
)

type productMCPIdentity struct{ scope gwWorkflow.MCPContext }

func (p *productMCPIdentity) Authorize(context.Context, gwWorkflow.MCPCredentials) (gwWorkflow.MCPContext, error) {
	return p.scope, nil
}

type productMCPCaller struct{ gateway *gwWorkflow.MCPGateway }

func (p productMCPCaller) Call(ctx context.Context, method string, params []byte) ([]byte, bool, error) {
	return p.gateway.Call(ctx, gwWorkflow.MCPCredentials{}, method, params)
}

func TestWorkspaceMCPBindingExplicitReleaseAndThreeAuthenticationModes(t *testing.T) {
	ctx := context.Background()
	controlDB, _ := database(t)
	gatewayDB, _ := databaseWithMigrations(t, "../../../media_agent_gateway/migrations")
	pluginDB, _ := databaseWithMigrations(t, "../../../media_plugin/migrations")
	authDB, _ := databaseWithMigrations(t, "../../../media_auth/migrations")
	facts := identityFacts(t)
	defer facts.Close()
	t.Setenv("MEDIA_AUTH_MCP_PLUGIN_TOKEN", "plugin-auth")
	oauthStore := oauthRepo.NewRepo(authDB)
	oauthHandler.McpOAuthSvc = oauthService.NewService(oauthStore, oauthClient.NewClient(nil), nil)
	authH := server.New()
	authH.POST("/internal/mcp-oauth/token", oauthHandler.GetUsableToken)
	authInternal := mount(authH)
	defer authInternal.Close()
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/me" {
			user := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if user == "" {
				w.WriteHeader(401)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": user})
			return
		}
		target := facts.URL
		if r.URL.Path == "/internal/mcp-oauth/token" {
			target = authInternal.URL
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, target+r.URL.RequestURI(), r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		req.Header = r.Header.Clone()
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer auth.Close()
	workspace := workspaceService.New(workspaceRepo.New(controlDB), dal.NewTxRunner(controlDB), secret.New("control"), authClient.New(auth.URL, "auth-internal", nil))
	workspaceHandler.Configure(workspace, "product-internal")
	middleware.Configure("", "product-internal")
	middleware.ConfigureMetadataGateway("metadata")
	controlH := server.New()
	workspaceRouter.Register(controlH)
	metadataRouter.Register(controlH)
	controlH.POST("/internal/workspaces:check", middleware.Session(), middleware.RequireInternal(), workspaceHandler.CheckWorkspaceAccess)
	control := mount(controlH)
	defer control.Close()
	must(t, controlDB.Exec("INSERT INTO workspaces(id,owner_user_id,created_by,organization_id,team_id,name,desired_state,version,created_at,updated_at) VALUES('workspace','owner','owner','org','workspace-team','Workspace','running',1,now(),now())").Error)
	must(t, controlDB.Exec("INSERT INTO agent_configurations(id,workspace_id,agent_id,agent_type,version,created_at,updated_at) VALUES('config','workspace','codex','codex',1,now(),now())").Error)
	var mu sync.Mutex
	seen := map[string]string{}
	upstream := mcp.NewServer(&mcp.Implementation{Name: "fixture", Version: "1"}, nil)
	upstream.AddTool(&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "working"}}}, nil
	})
	protocol := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return upstream }, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := strings.TrimPrefix(r.URL.Path, "/")
		actual := r.Header.Get("Authorization")
		if kind == "api_key" {
			actual = r.Header.Get("X-API-Key")
		}
		expected := map[string]string{"none": "", "bearer": "Bearer secret-one", "api_key": "api-secret", "oauth": "Bearer oauth-access"}[kind]
		if kind == "bearer" && actual == "Bearer secret-two" {
			expected = actual
		}
		if actual != expected || r.Header.Get("X-MCP-Gateway-Credential") != "" || r.Header.Get("X-Session-Token") != "" {
			t.Errorf("credential mismatch at %s", kind)
			w.WriteHeader(401)
			return
		}
		mu.Lock()
		seen[kind] = actual
		mu.Unlock()
		protocol.ServeHTTP(w, r)
	}))
	defer backend.Close()
	backendURL, err := url.Parse(backend.URL)
	must(t, err)
	catalog := catalogService.New(catalogRepo.New(gatewayDB), gwDAL.NewTxRunner(gatewayDB), []string{backendURL.Host})
	catalogHandler.Configure(catalog, nil)
	gwMiddleware.Configure("", "product-internal")
	gatewayH := server.New()
	catalogRouter.Register(gatewayH)
	gateway := mount(gatewayH)
	defer gateway.Close()
	for _, kind := range []string{"none", "bearer", "api_key", "oauth"} {
		_, err = catalog.PutScoped(ctx, catalogDomain.Definition{ID: kind, OrganizationID: "org", ServiceName: kind, Endpoint: backend.URL + "/" + kind, Transport: "streamable_http", AuthenticationType: kind, Status: "enabled"}, 0, "org")
		must(t, err)
	}
	must(t, authDB.Exec("INSERT INTO media_auth.users(id,username,display_name) VALUES('owner','owner','Owner')").Error)
	var refreshCalls atomic.Int32
	oauthAS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		must(t, r.ParseForm())
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh-secret" {
			t.Error("invalid OAuth refresh request")
			w.WriteHeader(400)
			return
		}
		refreshCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "oauth-access", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer oauthAS.Close()
	must(t, oauthStore.UpsertClient(ctx, oauthDomain.McpClient{ServerOrigin: backend.URL, ClientID: "oauth-client", Metadata: oauthDomain.Metadata{TokenEndpoint: oauthAS.URL}}))
	must(t, oauthStore.UpsertToken(ctx, "owner", backend.URL, oauthDomain.TokenResponse{AccessToken: "oauth-access", RefreshToken: "refresh-secret"}, 3600))
	must(t, authDB.Exec("UPDATE media_auth.mcp_oauth_tokens SET expires_at=now()-interval '1 minute' WHERE user_id='owner'").Error)
	authority := &pluginClient.Authority{ControlURL: control.URL, GatewayURL: gateway.URL, AuthURL: auth.URL, ControlToken: "product-internal", MetadataCredential: "metadata", AuthCredential: "plugin-auth"}
	bindings := &pluginWorkflow.Bindings{Service: bindingService.New(bindingRepo.New(pluginDB), pluginDAL.NewTxRunner(pluginDB)), Authority: authority, Secrets: pluginSecret.New("plugin-encryption")}
	pluginMiddleware.Configure(authority, "mcp-control", "mcp-gateway")
	bindingHandler.Configure(bindings)
	accessHandler.Configure(bindings)
	pluginH := server.New()
	bindingRouter.Register(pluginH)
	accessRouter.Register(pluginH)
	plugin := mount(pluginH)
	defer plugin.Close()
	artifacts, err := metadataartifact.New(t.TempDir())
	must(t, err)
	metadataHandler.Configure(metadataFlow.New(metadataService.New(metadataRepo.New(controlDB), dal.NewTxRunner(controlDB)), workspace, artifacts, nil).WithMCP(controlClient.MCPBindings{BaseURL: plugin.URL, Credential: "mcp-control"}))
	api := func(user, method, path string, body any, status int, out any) {
		t.Helper()
		raw, err := json.Marshal(body)
		must(t, err)
		req, err := http.NewRequest(method, plugin.URL+path, bytes.NewReader(raw))
		must(t, err)
		req.Header.Set("Authorization", "Bearer "+user)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		must(t, err)
		defer resp.Body.Close()
		payload, err := io.ReadAll(resp.Body)
		must(t, err)
		if resp.StatusCode != status {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, resp.StatusCode, status, payload)
		}
		if strings.Contains(string(payload), "secret-one") || strings.Contains(string(payload), "secret-two") || strings.Contains(string(payload), "api-secret") || strings.Contains(string(payload), "oauth-access") || strings.Contains(string(payload), "refresh-secret") {
			t.Fatal("secret leaked in browser response")
		}
		if out != nil {
			must(t, json.Unmarshal(payload, out))
		}
	}
	root := "/api/v1/workspaces/workspace/mcp-bindings"
	selected := []map[string]any{}
	created := map[string]bindingPB.Binding{}
	for _, kind := range []string{"none", "bearer", "api_key", "oauth"} {
		body := map[string]any{"definition_id": kind, "definition_version": 1, "enabled": true}
		if kind == "bearer" {
			body["credential"] = "secret-one"
		}
		if kind == "api_key" {
			body["credential"] = "api-secret"
			body["credential_header"] = "X-API-Key"
		}
		api("viewer", "POST", root, body, 403, nil)
		var value bindingPB.Binding
		api("owner", "POST", root, body, 201, &value)
		created[kind] = value
		selected = append(selected, map[string]any{"kind": "media_plugin.mcp_binding", "id": value.BindingId, "version": value.Version})
	}
	api("owner", "GET", "/internal/workspaces/workspace/mcp-bindings/"+created["bearer"].BindingId+"?version=1", nil, 403, nil)
	api("owner", "POST", "/internal/workspaces/workspace/mcp-access", map[string]any{"user_id": "owner"}, 403, nil)
	api("owner", "POST", root, map[string]any{"definition_id": "bearer", "definition_version": 1, "enabled": true, "credential": "orphan-must-rollback"}, 409, nil)
	api("owner", "POST", root, map[string]any{"definition_id": "api_key", "definition_version": 1, "enabled": true, "credential": "secret", "credential_header": "Host"}, 400, nil)
	api("outsider", "GET", root, nil, 403, nil)
	var selection metadataPB.MetadataContext
	confirm := func(version int64, key string) {
		call(t, control.URL, "owner", "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", map[string]any{"agent_config_id": "config", "expected_version": version, "idempotency_key": key, "resources": selected}, 200, &selection)
	}
	confirm(1, "first-mcp")
	identity := &productMCPIdentity{scope: gwWorkflow.MCPContext{OrganizationID: "org", WorkspaceID: "workspace", UserID: "owner", AgentConfigID: "config", SessionID: "session", ContextVersion: selection.ContextVersion, MetadataTargetContextVersion: selection.TargetContextVersion}}
	keys, err := sessioncredential.New("fixture", map[string][]byte{"fixture": bytes.Repeat([]byte{7}, 32)})
	must(t, err)
	flow := gwWorkflow.NewMCPGateway(identity, catalog, gwClient.PluginMCPAccess{BaseURL: plugin.URL, Credential: "mcp-gateway"}, gwClient.MCPUpstream{}, keys, nil)
	entry := httptest.NewServer(mcpprotocol.NewHandler(func(context.Context, http.Header) (mcpprotocol.Caller, int, error) {
		return productMCPCaller{flow}, 200, nil
	}))
	defer entry.Close()
	peer, err := mcp.NewClient(&mcp.Implementation{Name: "agent-fixture", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: entry.URL, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
	must(t, err)
	defer peer.Close()
	tools, err := peer.ListTools(ctx, nil)
	must(t, err)
	discovered := map[string]bool{}
	for _, tool := range tools.Tools {
		discovered[tool.Name] = true
	}
	for _, kind := range []string{"none", "bearer", "api_key", "oauth"} {
		if !discovered[kind+"__echo"] {
			t.Fatalf("missing discovered %s MCP", kind)
		}
	}
	for _, kind := range []string{"none", "bearer", "api_key", "oauth"} {
		result, err := peer.CallTool(ctx, &mcp.CallToolParams{Name: kind + "__echo", Arguments: map[string]any{}})
		must(t, err)
		if result.IsError || result.Content[0].(*mcp.TextContent).Text != "working" {
			t.Fatal(result)
		}
	}
	bearer := created["bearer"]
	body := map[string]any{"definition_id": "bearer", "definition_version": 1, "enabled": true, "expected_version": 1, "credential": "secret-two"}
	var updated bindingPB.Binding
	api("owner", "PUT", root+"/"+bearer.BindingId, body, 200, &updated)
	api("owner", "PUT", root+"/"+bearer.BindingId, body, 409, nil)
	var notices int64
	must(t, controlDB.Table("metadata_notification_outbox").Count(&notices).Error)
	if notices != 1 {
		t.Fatal("binding edit automatically published Agent configuration")
	}

	_, err = peer.CallTool(ctx, &mcp.CallToolParams{Name: "bearer__echo", Arguments: map[string]any{}})
	must(t, err)
	mu.Lock()
	oldSecret := seen["bearer"]
	mu.Unlock()
	if oldSecret != "Bearer secret-one" {
		t.Fatal("candidate edit changed fixed session")
	}
	for _, ref := range selected {
		if ref["id"] == bearer.BindingId {
			ref["version"] = updated.Version
		}
	}
	confirm(selection.SelectionRevision, "second-mcp")
	identity.scope.SessionID = "session-after-explicit-update"
	identity.scope.ContextVersion = selection.ContextVersion
	identity.scope.MetadataTargetContextVersion = selection.TargetContextVersion
	_, err = peer.CallTool(ctx, &mcp.CallToolParams{Name: "bearer__echo", Arguments: map[string]any{}})
	must(t, err)
	mu.Lock()
	newSecret := seen["bearer"]
	mu.Unlock()
	if newSecret != "Bearer secret-two" {
		t.Fatal("explicit update did not switch credential")
	}
	body["expected_version"] = 2
	body["enabled"] = false
	delete(body, "credential")
	api("owner", "PUT", root+"/"+bearer.BindingId, body, 200, nil)
	call(t, control.URL, "owner", "POST", "/api/v1/workspaces/workspace/agent-metadata:confirm", map[string]any{"agent_config_id": "config", "expected_version": selection.SelectionRevision, "idempotency_key": "disabled-selection", "resources": selected}, 422, nil)

	tools, err = peer.ListTools(ctx, nil)
	must(t, err)
	for _, tool := range tools.Tools {
		if tool.Name == "bearer__echo" {
			t.Fatal("disabled binding still callable")
		}
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("OAuth refresh calls=%d expected one", refreshCalls.Load())
	}
	must(t, oauthStore.DeleteToken(ctx, "owner", backend.URL))
	var connection bindingPB.Binding
	api("owner", "GET", root+"/"+created["oauth"].BindingId, nil, 200, &connection)
	if connection.ConnectionStatus != "needs_reauth" || !strings.HasPrefix(connection.OauthAuthorizeUrl, "/api/mcp-oauth/authorize?") {
		t.Fatal("missing reconnect action")
	}
	_, err = peer.CallTool(ctx, &mcp.CallToolParams{Name: "oauth__echo", Arguments: map[string]any{}})
	if err == nil {
		t.Fatal("OAuth disconnect still allowed calls")
	}
	var credentials []struct{ Ciphertext []byte }
	must(t, pluginDB.Table("media_plugin.mcp_credentials").Find(&credentials).Error)
	if len(credentials) != 3 {
		t.Fatal(fmt.Sprintf("credential history=%d", len(credentials)))
	}
	for _, credential := range credentials {
		if bytes.Contains(credential.Ciphertext, []byte("secret")) {
			t.Fatal("credential stored in plaintext")
		}
	}
	var history []struct{ Payload string }
	must(t, pluginDB.Table("media_plugin.plugin_binding_versions").Find(&history).Error)
	for _, version := range history {
		if strings.Contains(version.Payload, "secret-one") || strings.Contains(version.Payload, "secret-two") || strings.Contains(version.Payload, "api-secret") {
			t.Fatal("secret leaked in history")
		}
	}
}
