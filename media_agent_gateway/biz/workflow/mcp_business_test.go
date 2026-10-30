package workflow

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"media_agent/hertz_infra/authn/mcpdelegation"
	d "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type moduleAccessProbe struct {
	allowed     atomic.Bool
	unavailable bool
}

func (a *moduleAccessProbe) ModuleAllowed(context.Context, MCPContext, string) (bool, error) {
	if a.unavailable {
		return false, errors.New("offline")
	}
	return a.allowed.Load(), nil
}

type businessUpstreamProbe struct {
	opens     atomic.Int32
	callError error
}

func (a *businessUpstreamProbe) Open(context.Context, client.MCPBackend) (client.MCPPeer, error) {
	a.opens.Add(1)
	return businessPeer{callError: a.callError}, nil
}

type businessPeer struct{ callError error }

func (businessPeer) Close() error { return nil }
func (p businessPeer) Call(_ context.Context, method string, _ []byte) ([]byte, error) {
	if method == "tools/list" {
		return []byte(`{"tools":[{"name":"jobs_get","inputSchema":{"type":"object"}},{"name":"jobs_submit","inputSchema":{"type":"object"}}]}`), nil
	}
	return []byte(`{"content":[]}`), p.callError
}
func TestBusinessSourceFiltersBeforeConnectionAndRechecksOldToolNames(t *testing.T) {
	f, repo, bindings, identity := testMCPFlow(t, "http://unused.test")
	identity.scope.Kind = "user"
	identity.scope.ActorID = "User:owner"
	identity.scope.CredentialID = "credential"
	identity.scope.OrganizationID = "org"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpdelegation.NewSigner(key, "issuer", nil)
	if err != nil {
		t.Fatal(err)
	}
	access := &moduleAccessProbe{}
	f.WithBusiness(&MCPBusiness{DefinitionID: "media-workflow", Access: access, Signer: signer})
	upstream := &businessUpstreamProbe{}
	f.upstream = upstream
	repo.Save(context.Background(), &d.Definition{ID: "media-workflow", ServiceName: "media-workflow", Endpoint: "http://worker.test/mcp", Transport: "streamable_http", AuthenticationType: "session_context", Status: "enabled", Version: 1}, 0)
	// Users do not depend on a platform Session catalog at all.
	delete(repo.current, "platform")
	call := func(method string, params string) ([]byte, error) {
		raw, _, err := f.Call(context.Background(), MCPCredentials{}, method, []byte(params))
		return raw, err
	}
	raw, err := call("tools/list", `{}`)
	if err != nil || upstream.opens.Load() != 0 || bindings.calls != 0 {
		t.Fatalf("unauthorized source connected: %s %v", raw, err)
	}
	var list struct {
		Tools []json.RawMessage `json:"tools"`
	}
	json.Unmarshal(raw, &list)
	if len(list.Tools) != 0 {
		t.Fatal("unauthorized tools disclosed")
	}
	access.allowed.Store(true)
	raw, err = call("tools/list", `{}`)
	json.Unmarshal(raw, &list)
	if err != nil || len(list.Tools) != 2 {
		t.Fatalf("authorized tools missing %s %v", raw, err)
	}
	access.allowed.Store(false)
	before := upstream.opens.Load()
	if _, err = call("tools/call", `{"name":"media-workflow__jobs_submit","arguments":{"items":[]}}`); err == nil || upstream.opens.Load() != before {
		t.Fatal("old tool name bypassed revocation")
	}
	access.unavailable = true
	if _, err = call("tools/list", `{}`); !errors.Is(err, ErrMCPUnavailable) {
		t.Fatal("authority outage presented as empty permission", err)
	}
}

func TestManagedBusinessSourceCannotBypassExplicitBindingRestrictions(t *testing.T) {
	f, repo, bindings, identity := testMCPFlow(t, "http://unused.test")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpdelegation.NewSigner(key, "issuer", nil)
	if err != nil {
		t.Fatal(err)
	}
	access := &moduleAccessProbe{}
	access.allowed.Store(true)
	f.WithBusiness(&MCPBusiness{DefinitionID: "media-workflow", Access: access, Signer: signer})
	f.upstream = &businessUpstreamProbe{}
	repo.Save(context.Background(), &d.Definition{ID: "media-workflow", ServiceName: "media-workflow", Endpoint: "http://worker.test/mcp", Transport: "streamable_http", AuthenticationType: "session_context", Status: "enabled", Version: 1}, 0)
	bindings.value.Grants = nil
	bindings.value.RestrictedDefinitionIDs = []string{"media-workflow"}
	sources, _, err := f.sources(context.Background(), identity.scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.definitionID == "media-workflow" {
			t.Fatal("disabled binding was restored as automatic business source")
		}
	}
	bindings.value.Grants = []client.MCPGrant{{DefinitionID: "media-workflow", DefinitionVersion: 1, AllowedMethods: []string{"tools/list", "tools/call"}, AllowedTools: []string{"jobs_get"}}}
	sources, _, err = f.sources(context.Background(), identity.scope)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range sources {
		if source.definitionID == "media-workflow" {
			found = true
			entries, err := f.sourceList(context.Background(), source, "tools/list")
			if err != nil || len(entries) != 1 || entries[0]["name"] != "media-workflow__jobs_get" {
				t.Fatalf("binding tool restrictions lost: %+v %v", entries, err)
			}
		}
	}
	if !found {
		t.Fatal("enabled source missing")
	}
	bindings.value.Grants[0].DefinitionVersion = 2
	if _, _, err = f.sources(context.Background(), identity.scope); !errors.Is(err, ErrMCPUnavailable) {
		t.Fatal("fixed binding silently moved to another version")
	}
}

func TestMCPRemoteRejectionsRemainDefinite(t *testing.T) {
	for _, tc := range []struct {
		code int64
		want error
	}{{-32602, ErrMCPArgument}, {-32601, ErrMCPArgument}, {-32001, ErrMCPForbidden}} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			f, _, _, _ := testMCPFlow(t, "http://unused.test")
			f.upstream = &businessUpstreamProbe{callError: &client.MCPRemoteError{Code: tc.code}}
			_, _, err := f.Call(context.Background(), MCPCredentials{}, "tools/call", []byte(`{"name":"alpha__jobs_submit","arguments":{"items":[]}}`))
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

type moduleMatrixAccess map[string]bool

func (m moduleMatrixAccess) ModuleAllowed(_ context.Context, _ MCPContext, module string) (bool, error) {
	return m[module], nil
}
func TestMaterialsSourceDoesNotRequireWorkflowAndRechecksRevocation(t *testing.T) {
	f, repo, _, identity := testMCPFlow(t, "http://unused.test")
	identity.scope.Kind = "user"
	identity.scope.ActorID = "User:owner"
	identity.scope.OrganizationID = "org"
	identity.scope.CredentialID = "credential"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := mcpdelegation.NewSigner(key, "issuer", nil)
	if err != nil {
		t.Fatal(err)
	}
	access := moduleMatrixAccess{"materials": true, "workflow": false}
	f.WithBusiness(&MCPBusiness{DefinitionID: "media-workflow", MaterialsDefinitionID: "media-materials", Access: access, Signer: signer})
	for _, id := range []string{"media-workflow", "media-materials"} {
		repo.Save(context.Background(), &d.Definition{ID: id, ServiceName: id, Endpoint: "http://worker.test/mcp", Transport: "streamable_http", AuthenticationType: "session_context", Status: "enabled", Version: 1}, 0)
	}
	sources, _, err := f.sources(context.Background(), identity.scope)
	if err != nil || len(sources) != 1 || sources[0].definitionID != "media-materials" {
		t.Fatalf("materials-only access failed: %v %v", sources, err)
	}
	headers, err := sources[0].backend.RequestHeaders(context.Background(), "tools/call", json.RawMessage(`{"name":"virtual_ips_list","arguments":{"items":[{"item_id":"list","params":{}}]}}`))
	if err != nil || headers[mcpdelegation.MaterialsHeader] == "" || headers[mcpdelegation.WorkflowHeader] != "" {
		t.Fatal("materials source obtained wrong target credentials", err)
	}
	access["materials"] = false
	upstream := &businessUpstreamProbe{}
	f.upstream = upstream
	_, _, err = f.Call(context.Background(), MCPCredentials{}, "tools/call", []byte(`{"name":"media-materials__virtual_ips_list","arguments":{"items":[]}}`))
	if !errors.Is(err, ErrMCPForbidden) || upstream.opens.Load() != 0 {
		t.Fatal("revoked source connected or accepted stale tool", err)
	}
}
