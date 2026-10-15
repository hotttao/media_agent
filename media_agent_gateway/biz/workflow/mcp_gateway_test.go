package workflow

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"media_agent/hertz_infra/mcpprotocol"
	"media_agent/hertz_infra/sessioncredential"
	d "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
	catalog "media_agent/media_agent_gateway/biz/service/mcp_catalog"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type mcpMemoryCatalog struct {
	mu      sync.Mutex
	current map[string]d.Definition
	history map[string]d.Definition
}

func (r *mcpMemoryCatalog) Get(_ context.Context, id string, _ bool) (*d.Definition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.current[id]
	if !ok {
		return nil, d.ErrNotFound
	}
	return &v, nil
}
func (r *mcpMemoryCatalog) Version(_ context.Context, id string, v int64) (*d.Definition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.history[fmt.Sprintf("%s:%d", id, v)]
	if !ok {
		return nil, d.ErrNotFound
	}
	return &value, nil
}
func (r *mcpMemoryCatalog) List(context.Context, string, int) ([]d.Definition, error) {
	return nil, nil
}
func (r *mcpMemoryCatalog) Save(_ context.Context, v *d.Definition, _ int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current[v.ID] = *v
	r.history[fmt.Sprintf("%s:%d", v.ID, v.Version)] = *v
	return nil
}
func (r *mcpMemoryCatalog) Changed(context.Context, d.Event) error { return nil }

type mcpTestTx struct{}

func (mcpTestTx) RunInTx(ctx context.Context, f func(context.Context) error) error { return f(ctx) }

type mcpTestIdentity struct {
	scope   MCPContext
	revoked atomic.Bool
}

func (i *mcpTestIdentity) Authorize(context.Context, MCPCredentials) (MCPContext, error) {
	if i.revoked.Load() {
		return MCPContext{}, ErrMCPForbidden
	}
	return i.scope, nil
}

type mcpTestAccess struct {
	mu       sync.Mutex
	value    client.MCPAccess
	calls    int
	revokeAt int
}

func (a *mcpTestAccess) Resolve(_ context.Context, scope MCPContext) (client.MCPAccess, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	v := a.value
	if scope.ContextVersion != 7 {
		return v, errors.New("wrong fixed context")
	}
	if a.revokeAt > 0 && a.calls >= a.revokeAt {
		v.Grants = nil
	}
	return v, nil
}

type mcpTestCaller struct{ flow *MCPGateway }

func (c mcpTestCaller) Call(ctx context.Context, method string, params []byte) ([]byte, bool, error) {
	return c.flow.Call(ctx, MCPCredentials{}, method, params)
}

type mcpHeaders struct{ headers http.Header }

func (t mcpHeaders) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	for k, v := range t.headers {
		r.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(r)
}
func mcpTestClient(t *testing.T, url string) *mcp.ClientSession {
	t.Helper()
	httpClient := &http.Client{Transport: mcpHeaders{http.Header{"X-Session-Token": {"session-secret"}}}}
	p, e := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true}, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { p.Close() })
	return p
}
func testMCPBackend(t *testing.T, count int, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	s := mcp.NewServer(&mcp.Implementation{Name: "backend", Version: "1"}, &mcp.ServerOptions{PageSize: 23})
	for n := 0; n < count; n++ {
		name := fmt.Sprintf("echo_%03d", n)
		s.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if bytes.Contains(r.Params.Arguments, []byte("number")) && !bytes.Contains(r.Params.Arguments, []byte("9007199254740993")) {
				t.Errorf("integer arguments lost precision: %s", r.Params.Arguments)
			}
			calls.Add(1)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: r.Params.Name}}}, nil
		})
	}
	s.AddPrompt(&mcp.Prompt{Name: "instructions"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "backend prompt"}}}}, nil
	})
	read := func(_ context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: r.Params.URI, Text: "resource body"}}}, nil
	}
	s.AddResource(&mcp.Resource{Name: "document", URI: "notes://doc?q=%2F", MIMEType: "text/plain"}, read)
	s.AddResourceTemplate(&mcp.ResourceTemplate{Name: "document", URITemplate: "notes://doc/{id}"}, read)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer backend-delegation" || r.Header.Get("X-Session-Token") != "" || r.Header.Get("X-Media-User-ID") != "owner" {
			t.Errorf("backend received wrong identity or entry credential")
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}
func testMCPFlow(t *testing.T, endpoint string) (*MCPGateway, *mcpMemoryCatalog, *mcpTestAccess, *mcpTestIdentity) {
	t.Helper()
	defs := []d.Definition{{ID: "platform", ServiceName: "platform", Endpoint: "platform://builtin", Transport: "platform", AuthenticationType: "session_context", Status: "enabled", Version: 1}, {ID: "alpha", ServiceName: "alpha", Endpoint: endpoint, Transport: "streamable_http", AuthenticationType: "bearer", Status: "enabled", Version: 1}, {ID: "beta", ServiceName: "beta", Endpoint: endpoint, Transport: "streamable_http", AuthenticationType: "bearer", Status: "enabled", Version: 1}}
	repo := &mcpMemoryCatalog{current: map[string]d.Definition{}, history: map[string]d.Definition{}}
	for _, v := range defs {
		repo.Save(context.Background(), &v, 0)
	}
	identity := &mcpTestIdentity{scope: MCPContext{SessionID: "s1", UserID: "owner", WorkspaceID: "ws", AgentConfigID: "cfg", ContextVersion: 7, MetadataTargetContextVersion: "release-seven"}}
	access := &mcpTestAccess{value: client.MCPAccess{SnapshotVersion: "release-seven", AuthorizationRevision: "acl-one"}}
	for _, id := range []string{"alpha", "beta"} {
		access.value.Grants = append(access.value.Grants, client.MCPGrant{DefinitionID: id, DefinitionVersion: 1, AllowedMethods: []string{"*"}, AllowedTools: []string{"*"}, AllowedPrompts: []string{"*"}, AllowedResources: []string{"notes://doc?q=%2F", "notes://doc/{id}"}, ProxyAuthorization: "Bearer backend-delegation"})
	}
	keys, e := sessioncredential.New("test", map[string][]byte{"test": bytes.Repeat([]byte{9}, 32)})
	if e != nil {
		t.Fatal(e)
	}
	flow := NewMCPGateway(identity, catalog.New(repo, mcpTestTx{}, nil), access, client.MCPUpstream{}, keys, nil)
	return flow, repo, access, identity
}
func testMCPGatewayHTTP(t *testing.T, flow *MCPGateway) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(mcpprotocol.NewHandler(func(_ context.Context, h http.Header) (mcpprotocol.Caller, int, error) {
		if h.Get("X-Session-Token") != "session-secret" {
			return nil, 401, ErrMCPForbidden
		}
		return mcpTestCaller{flow}, 200, nil
	}))
	t.Cleanup(server.Close)
	return server
}
func TestMCPProtocolPaginationAcrossGatewaysAndSources(t *testing.T) {
	var calls atomic.Int32
	backend := testMCPBackend(t, 65, &calls)
	flow, repo, access, identity := testMCPFlow(t, backend.URL)
	sources, _, debugErr := flow.sources(context.Background(), identity.scope)
	if debugErr != nil {
		t.Fatal(debugErr)
	}
	for _, source := range sources {
		items, e := flow.sourceList(context.Background(), source, "tools/list")
		if e != nil {
			t.Fatalf("source %s: %v items=%d", source.definitionID, e, len(items))
		}
	}
	first, second := mcpTestClient(t, testMCPGatewayHTTP(t, flow).URL), mcpTestClient(t, testMCPGatewayHTTP(t, NewMCPGateway(identity, flow.catalog, access, client.MCPUpstream{}, flow.keys, nil)).URL)
	ctx := context.Background()
	page, e := first.ListTools(ctx, nil)
	if e != nil || len(page.Tools) != 100 || page.NextCursor == "" {
		t.Fatalf("first page: %+v %v", page, e)
	}
	access.mu.Lock()
	for n := range access.value.Grants {
		slices.Reverse(access.value.Grants[n].AllowedResources)
	}
	access.mu.Unlock()
	tail, e := second.ListTools(ctx, &mcp.ListToolsParams{Cursor: page.NextCursor})
	if e != nil || len(tail.Tools) != 32 || tail.NextCursor != "" {
		t.Fatalf("cross-Gateway continuation: %+v %v", tail, e)
	}
	tool, e := second.CallTool(ctx, &mcp.CallToolParams{Name: "beta__echo_001", Arguments: json.RawMessage(`{"number":9007199254740993}`)})
	if e != nil || tool.Content[0].(*mcp.TextContent).Text != "echo_001" || calls.Load() != 1 {
		t.Fatalf("tool mapping: %+v %v", tool, e)
	}
	prompts, e := first.ListPrompts(ctx, nil)
	if e != nil || len(prompts.Prompts) != 3 {
		t.Fatalf("prompts: %+v %v", prompts, e)
	}
	prompt, e := first.GetPrompt(ctx, &mcp.GetPromptParams{Name: "alpha__instructions"})
	if e != nil || prompt.Messages[0].Content.(*mcp.TextContent).Text != "backend prompt" {
		t.Fatalf("prompt: %+v %v", prompt, e)
	}
	resources, e := first.ListResources(ctx, nil)
	if e != nil || len(resources.Resources) != 3 {
		t.Fatalf("resources: %+v %v", resources, e)
	}
	templates, e := first.ListResourceTemplates(ctx, nil)
	if e != nil || len(templates.ResourceTemplates) != 2 || templates.ResourceTemplates[0].URITemplate != "mcp-gateway://alpha/notes://doc/{id}" {
		t.Fatalf("templates: %+v %v", templates, e)
	}
	for _, uri := range []string{"mcp-gateway://alpha/notes://doc?q=%2F", "mcp-gateway://alpha/notes://doc/value"} {
		read, e := first.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if e != nil || read.Contents[0].URI != uri {
			t.Fatalf("resource read: %+v %v", read, e)
		}
	}
	access.mu.Lock()
	access.value.AuthorizationRevision = "acl-two"
	access.mu.Unlock()
	if _, e = second.ListTools(ctx, &mcp.ListToolsParams{Cursor: page.NextCursor}); e == nil {
		t.Fatal("cursor survived ACL revision change")
	}
	repo.mu.Lock()
	definition := repo.current["beta"]
	definition.Status = "disabled"
	definition.Version = 2
	repo.current["beta"] = definition
	repo.mu.Unlock()
	if _, e = first.CallTool(ctx, &mcp.CallToolParams{Name: "beta__echo_001"}); e == nil || calls.Load() != 1 {
		t.Fatal("disabled definition executed")
	}
	identity.revoked.Store(true)
	if _, e = first.ListTools(ctx, nil); e == nil {
		t.Fatal("revoked identity could discover")
	}
}
func TestMCPRejectsRevocationAfterDiscoveryAndSpoofedScope(t *testing.T) {
	var calls atomic.Int32
	backend := testMCPBackend(t, 1, &calls)
	flow, _, access, _ := testMCPFlow(t, backend.URL)
	access.revokeAt = 2
	_, _, e := flow.Call(context.Background(), MCPCredentials{}, "tools/call", []byte(`{"name":"alpha__echo_000"}`))
	if !errors.Is(e, ErrMCPForbidden) || calls.Load() != 0 {
		t.Fatalf("revocation window: %v calls=%d", e, calls.Load())
	}
	for _, raw := range []string{`{"name":"platform__session_context","arguments":{"workspace_id":"other"}}`, `{"name":"platform__session_context","session_id":"other"}`} {
		_, _, e = flow.Call(context.Background(), MCPCredentials{}, "tools/call", []byte(raw))
		if !errors.Is(e, ErrMCPArgument) {
			t.Fatal(e)
		}
	}
}

type mcpFailPeer struct{ calls *atomic.Int32 }

func (p mcpFailPeer) Close() error { return nil }
func (p mcpFailPeer) Call(_ context.Context, method string, _ []byte) ([]byte, error) {
	if method == "tools/list" {
		return []byte(`{"tools":[{"name":"write","inputSchema":{"type":"object"}}]}`), nil
	}
	p.calls.Add(1)
	return nil, client.ErrMCPUpstream
}

type mcpFailUpstream struct{ calls *atomic.Int32 }

func (p mcpFailUpstream) Open(context.Context, client.MCPBackend) (client.MCPPeer, error) {
	return mcpFailPeer{p.calls}, nil
}
func TestMCPUnknownWriteIsNeverRepeatedAndCursorIsScoped(t *testing.T) {
	flow, _, _, _ := testMCPFlow(t, "http://unused")
	var calls atomic.Int32
	flow.upstream = mcpFailUpstream{&calls}
	_, _, e := flow.Call(context.Background(), MCPCredentials{}, "tools/call", []byte(`{"name":"alpha__write"}`))
	if !errors.Is(e, ErrMCPOutcomeUnknown) || calls.Load() != 1 {
		t.Fatalf("write replayed: %v %d", e, calls.Load())
	}
	entries := make([]map[string]any, 101)
	for i := range entries {
		entries[i] = map[string]any{"name": fmt.Sprint(i)}
	}
	scope := MCPContext{SessionID: "one"}
	_, cursor, e := flow.page(scope, "tools/list", "v", "", entries)
	if e != nil {
		t.Fatal(e)
	}
	for _, variant := range []struct {
		scope                  MCPContext
		method, vector, cursor string
	}{{MCPContext{SessionID: "two"}, "tools/list", "v", cursor}, {scope, "prompts/list", "v", cursor}, {scope, "tools/list", "changed", cursor}, {scope, "tools/list", "v", cursor + "x"}} {
		if _, _, e = flow.page(variant.scope, variant.method, variant.vector, variant.cursor, entries); !errors.Is(e, ErrMCPCursor) {
			t.Fatalf("bad cursor accepted: %v", e)
		}
	}
}

type mcpMetadata struct {
	value    client.MetadataContext
	artifact client.MetadataArtifact
}

func (m mcpMetadata) Get(_ context.Context, user string, q client.MetadataQuery) (client.MetadataContext, error) {
	if user != "owner" || q.TargetContextVersion != "release-seven" || !q.ForSession {
		return client.MetadataContext{}, ErrMetadataForbidden
	}
	return m.value, nil
}
func (m mcpMetadata) Report(context.Context, string, client.MetadataResult) error { return nil }
func (m mcpMetadata) Artifact(context.Context, string, client.MetadataQuery, string) (client.MetadataArtifact, error) {
	return m.artifact, nil
}
func TestPlatformPromptAndSkillAreFixedToSession(t *testing.T) {
	flow, _, _, _ := testMCPFlow(t, "http://unused")
	flow.bindings = nil
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	file, _ := z.Create("SKILL.md")
	file.Write([]byte("Selected Skill body"))
	z.Close()
	sum := sha256.Sum256(buf.Bytes())
	checksum := hex.EncodeToString(sum[:])
	value := client.MetadataContext{MetadataSchemaVersion: 1, WorkspaceID: "ws", AgentConfigID: "cfg", ContextVersion: 7, TargetContextVersion: "release-seven", PlatformPromptTemplateVersion: "2", ConfiguredPrompt: "private configured instructions", Resources: []client.MetadataResource{{Kind: "media_control.skill", ID: "selected", Name: "Selected", Version: 7, Checksum: checksum}}}
	flow.metadata = mcpMetadata{value, client.MetadataArtifact{Content: buf.Bytes(), Checksum: checksum}}
	raw, _, e := flow.Call(context.Background(), MCPCredentials{}, "prompts/get", []byte(`{"name":"platform__platform_instructions"}`))
	if e != nil || strings.Contains(string(raw), value.ConfiguredPrompt) || !strings.Contains(string(raw), "Selected") {
		t.Fatalf("prompt body: %s %v", raw, e)
	}
	raw, _, e = flow.Call(context.Background(), MCPCredentials{}, "tools/call", []byte(`{"name":"platform__read_skill","arguments":{"skill_id":"selected"}}`))
	if e != nil || !strings.Contains(string(raw), "Selected Skill body") {
		t.Fatalf("skill body: %s %v", raw, e)
	}
	_, _, e = flow.Call(context.Background(), MCPCredentials{}, "tools/call", []byte(`{"name":"platform__read_skill","arguments":{"skill_id":"other"}}`))
	if !errors.Is(e, ErrMCPForbidden) {
		t.Fatal("unselected skill accepted", e)
	}
	legacy, version, e := RenderPlatformPrompt(client.MetadataContext{WorkspaceID: "ws"})
	if e != nil || version != "1" || legacy != "You are working in workspace ws.\nOnly /workspace persists. Keep work inside /workspace.\nRuntime configuration and skills are pinned to this session's selected version.\n" {
		t.Fatal("legacy template changed")
	}
	value.PlatformPromptTemplateVersion = "future"
	if _, _, e = RenderPlatformPrompt(value); !errors.Is(e, ErrPromptTemplate) {
		t.Fatal("unknown template fell back")
	}
}

func TestMCPRejectsForeignEnterpriseEvenWithPluginGrant(t *testing.T) {
	flow, repo, _, identity := testMCPFlow(t, "https://example.test/mcp")
	value := repo.current["alpha"]
	value.OrganizationID = "enterprise"
	if err := repo.Save(context.Background(), &value, 0); err != nil {
		t.Fatal(err)
	}
	for _, organization := range []string{"", "foreign"} {
		identity.scope.OrganizationID = organization
		if _, _, err := flow.sources(context.Background(), identity.scope); !errors.Is(err, ErrMCPUnavailable) {
			t.Fatalf("foreign grant accepted for %q: %v", organization, err)
		}
	}
	identity.scope.OrganizationID = "enterprise"
	if _, _, err := flow.sources(context.Background(), identity.scope); err != nil {
		t.Fatal(err)
	}
	// Corrupt or mismatched history cannot move a pinned source across enterprises.
	value.OrganizationID = "foreign"
	repo.history["alpha:1"] = value
	if _, _, err := flow.sources(context.Background(), identity.scope); !errors.Is(err, ErrMCPUnavailable) {
		t.Fatal(err)
	}
}
