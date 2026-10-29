package workflow

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/yosida95/uritemplate/v3"
	"io"
	"log/slog"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/hertz_infra/sessioncredential"
	catalog "media_agent/media_agent_gateway/biz/service/mcp_catalog"
	"media_agent/media_agent_gateway/biz/shared/client"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrMCPCursor = errors.New("MCP cursor invalid or source version changed")
var ErrMCPArgument = errors.New("invalid MCP parameters")
var ErrMCPOutcomeUnknown = errors.New("MCP result unknown; do not repeat a write blindly")

type MCPIdentityPort interface {
	Authorize(context.Context, MCPCredentials) (MCPContext, error)
}
type MCPBindingAccess interface {
	Resolve(context.Context, MCPContext) (client.MCPAccess, error)
}
type MCPUpstreamPort interface {
	Open(context.Context, client.MCPBackend) (client.MCPPeer, error)
}
type MCPGateway struct {
	identity MCPIdentityPort
	catalog  *catalog.Service
	bindings MCPBindingAccess
	upstream MCPUpstreamPort
	keys     *sessioncredential.Ring
	metadata MetadataControl
	business *MCPBusiness
}

func NewMCPGateway(identity MCPIdentityPort, c *catalog.Service, bindings MCPBindingAccess, upstream MCPUpstreamPort, keys *sessioncredential.Ring, metadata MetadataControl) *MCPGateway {
	return &MCPGateway{identity: identity, catalog: c, bindings: bindings, upstream: upstream, keys: keys, metadata: metadata}
}

type mcpSource struct {
	definitionID            string
	version, currentVersion int64
	grant                   client.MCPGrant
	backend                 client.MCPBackend
}

func allowed(values []string, name string) bool {
	return slices.Contains(values, "*") || slices.Contains(values, name)
}
func (w *MCPGateway) sources(ctx context.Context, scope MCPContext) ([]mcpSource, string, error) {
	var e error
	sources := []mcpSource{}
	if scope.Kind != "user" {
		platform, err := w.catalog.Get(ctx, "platform")
		if err != nil {
			return nil, "", ErrMCPUnavailable
		}
		if platform.Status == "enabled" {
			sources = append(sources, mcpSource{definitionID: "platform", version: platform.Version, currentVersion: platform.Version})
		}
	}
	access := client.MCPAccess{SnapshotVersion: "platform-only", AuthorizationRevision: "platform-only"}
	if w.bindings != nil && scope.Kind != "user" {
		access, e = w.bindings.Resolve(ctx, scope)
		if e != nil {
			return nil, "", ErrMCPUnavailable
		}
	}
	if len(access.Grants) > 128 || access.SnapshotVersion == "" || access.AuthorizationRevision == "" {
		return nil, "", ErrMCPUnavailable
	}
	business, err := w.businessSource(ctx, scope)
	if err != nil {
		return nil, "", err
	}
	if business != nil && slices.Contains(access.RestrictedDefinitionIDs, business.definitionID) {
		granted := false
		for _, grant := range access.Grants {
			if grant.DefinitionID == business.definitionID {
				granted = true
				break
			}
		}
		if !granted {
			business = nil
		}
	}
	seen := map[string]bool{"platform": true}
	if business != nil {
		sources = append(sources, *business)
	}
	for _, grant := range access.Grants {
		if w.business != nil && grant.DefinitionID == w.business.DefinitionID {
			// A managed binding may only narrow the built-in business source.
			// Do not silently apply a fixed binding to a different definition.
			if seen[grant.DefinitionID] || grant.DefinitionVersion < 1 {
				return nil, "", ErrMCPUnavailable
			}
			seen[grant.DefinitionID] = true
			if business != nil && grant.DefinitionVersion != business.version {
				return nil, "", ErrMCPUnavailable
			}
			if business != nil {
				for i := range sources {
					if sources[i].definitionID == grant.DefinitionID {
						sources[i].grant = grant
					}
				}
			}
			continue
		}
		if seen[grant.DefinitionID] || grant.DefinitionVersion < 1 {
			return nil, "", ErrMCPUnavailable
		}
		seen[grant.DefinitionID] = true
		current, e := w.catalog.Get(ctx, grant.DefinitionID)
		if e != nil {
			return nil, "", ErrMCPUnavailable
		}
		if current.OrganizationID != "" && current.OrganizationID != scope.OrganizationID {
			return nil, "", ErrMCPUnavailable
		}
		if current.Status != "enabled" {
			continue
		}
		fixed, e := w.catalog.Version(ctx, grant.DefinitionID, grant.DefinitionVersion)
		if e != nil || fixed.Status != "enabled" || fixed.Transport == "platform" || fixed.OrganizationID != current.OrganizationID {
			return nil, "", ErrMCPUnavailable
		}
		if fixed.AuthenticationType == "oauth" && grant.ProxyEndpoint == "" || grant.ProxyEndpoint != "" && fixed.AuthenticationType != "oauth" {
			return nil, "", ErrMCPUnavailable
		}
		if fixed.AuthenticationType != "none" && grant.ProxyAuthorization == "" && len(grant.UpstreamHeaders) == 0 {
			return nil, "", ErrMCPUnavailable
		}
		headers := map[string]string{}
		for key, value := range grant.UpstreamHeaders {
			headers[key] = value
		}
		for key, value := range map[string]string{"X-Media-Context-Version": strconv.FormatInt(scope.ContextVersion, 10), "X-Media-Metadata-Target": scope.MetadataTargetContextVersion, "X-Media-Workspace-ID": scope.WorkspaceID, "X-Media-Session-ID": scope.SessionID, "X-Media-User-ID": scope.UserID, "X-Media-Agent-Configuration-ID": scope.AgentConfigID} {
			headers[key] = value
		}
		if grant.ProxyAuthorization != "" {
			headers["Authorization"] = grant.ProxyAuthorization
		}
		endpoint, transport := fixed.Endpoint, fixed.Transport
		if grant.ProxyEndpoint != "" {
			endpoint, transport = grant.ProxyEndpoint, "streamable_http"
		}
		sources = append(sources, mcpSource{definitionID: grant.DefinitionID, version: fixed.Version, currentVersion: current.Version, grant: grant, backend: client.MCPBackend{Endpoint: endpoint, Transport: transport, Headers: headers}})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].definitionID < sources[j].definitionID })
	vector := access.SnapshotVersion + ":" + access.AuthorizationRevision
	for _, source := range sources {
		raw, _ := json.Marshal(struct {
			ID                                 string
			Version, Current                   int64
			Methods, Tools, Prompts, Resources []string
		}{source.definitionID, source.version, source.currentVersion, canonicalACL(source.grant.AllowedMethods), canonicalACL(source.grant.AllowedTools), canonicalACL(source.grant.AllowedPrompts), canonicalACL(source.grant.AllowedResources)})
		vector += string(raw)
	}
	return sources, vector, nil
}
func publicName(source, name string) string {
	result := source + "__" + name
	if len(result) <= 128 {
		return result
	}
	sum := sha256.Sum256([]byte(name))
	limit := 128 - len(source) - 2 - 17
	for limit > 0 && !utf8.ValidString(name[:limit]) {
		limit--
	}
	return source + "__" + name[:limit] + "_" + hex.EncodeToString(sum[:8])
}
func resourceURI(source, uri string) string { return "mcp-gateway://" + source + "/" + uri }
func rewriteResourceReferences(value any, source string) {
	switch v := value.(type) {
	case map[string]any:
		kind, _ := v["type"].(string)
		if kind == "resource_link" {
			if uri, ok := v["uri"].(string); ok {
				v["uri"] = resourceURI(source, uri)
			}
		}
		if kind == "resource" {
			if resource, ok := v["resource"].(map[string]any); ok {
				if uri, ok := resource["uri"].(string); ok {
					resource["uri"] = resourceURI(source, uri)
				}
			}
		}
		for _, child := range v {
			rewriteResourceReferences(child, source)
		}
	case []any:
		for _, child := range v {
			rewriteResourceReferences(child, source)
		}
	}
}
func listField(method string) string {
	switch method {
	case "tools/list":
		return "tools"
	case "prompts/list":
		return "prompts"
	case "resources/list":
		return "resources"
	case "resources/templates/list":
		return "resourceTemplates"
	}
	return ""
}
func (w *MCPGateway) sourceList(ctx context.Context, source mcpSource, method string) ([]map[string]any, error) {
	if source.definitionID == "platform" {
		return w.platformList(method), nil
	}
	if !allowed(source.grant.AllowedMethods, method) {
		return []map[string]any{}, nil
	}
	peer, e := w.upstream.Open(ctx, source.backend)
	if e != nil {
		if errors.Is(e, client.ErrMCPReauth) {
			return nil, &MCPReauth{Source: source.definitionID}
		}
		return nil, ErrMCPUnavailable
	}
	defer peer.Close()
	return w.sourceListPeer(ctx, source, method, peer)
}
func (w *MCPGateway) sourceListPeer(ctx context.Context, source mcpSource, method string, peer client.MCPPeer) ([]map[string]any, error) {
	if !allowed(source.grant.AllowedMethods, method) {
		return []map[string]any{}, nil
	}
	cursor := ""
	seen := map[string]bool{}
	entries := []map[string]any{}
	for page := 0; page < 64; page++ {
		params, _ := json.Marshal(map[string]any{"cursor": cursor})
		raw, e := peer.Call(ctx, method, params)
		if e != nil {
			if errors.Is(e, client.ErrMCPReauth) {
				return nil, &MCPReauth{Source: source.definitionID}
			}
			return nil, ErrMCPUnavailable
		}
		var result map[string]json.RawMessage
		if json.Unmarshal(raw, &result) != nil {
			return nil, ErrMCPUnavailable
		}
		var items []map[string]any
		if json.Unmarshal(result[listField(method)], &items) != nil {
			return nil, ErrMCPUnavailable
		}
		for _, item := range items {
			name, _ := item["name"].(string)
			item["_media_origin_name"] = name
			switch method {
			case "tools/list":
				if name == "" || !allowed(source.grant.AllowedTools, name) {
					continue
				}
				item["name"] = publicName(source.definitionID, name)
			case "prompts/list":
				if name == "" || !allowed(source.grant.AllowedPrompts, name) {
					continue
				}
				item["name"] = publicName(source.definitionID, name)
			case "resources/list", "resources/templates/list":
				key := "uri"
				if method == "resources/templates/list" {
					key = "uriTemplate"
				}
				uri, _ := item[key].(string)
				if uri == "" || !allowed(source.grant.AllowedResources, uri) {
					continue
				}
				item[key] = resourceURI(source.definitionID, uri)
				if name != "" {
					item["name"] = publicName(source.definitionID, name)
				}
			}
			entries = append(entries, item)
			if len(entries) > 2000 {
				return nil, ErrMCPUnavailable
			}
		}
		next := ""
		if raw := result["nextCursor"]; len(raw) > 0 && json.Unmarshal(raw, &next) != nil {
			return nil, ErrMCPUnavailable
		}
		if next == "" {
			return entries, nil
		}
		if seen[next] {
			return nil, ErrMCPUnavailable
		}
		seen[next] = true
		cursor = next
	}
	return nil, ErrMCPUnavailable
}

type mcpCursor struct {
	Session, Method, Vector, Key string
	Offset                       int
	Issued                       int64
}

func (w *MCPGateway) page(scope MCPContext, method, vector, cursor string, entries []map[string]any) ([]map[string]any, string, error) {
	offset := 0
	if cursor != "" {
		if len(cursor) > 4096 {
			return nil, "", ErrMCPCursor
		}
		parts := strings.Split(cursor, ".")
		if len(parts) != 2 {
			return nil, "", ErrMCPCursor
		}
		raw, e := base64.RawURLEncoding.DecodeString(parts[0])
		if e != nil {
			return nil, "", ErrMCPCursor
		}
		var c mcpCursor
		if json.Unmarshal(raw, &c) != nil {
			return nil, "", ErrMCPCursor
		}
		provided, e := base64.RawURLEncoding.DecodeString(parts[1])
		if e != nil {
			return nil, "", ErrMCPCursor
		}
		signature, e := w.keys.Derive("mc1", c.Key, parts[0])
		if e != nil || subtle.ConstantTimeCompare([]byte(signature), provided) != 1 {
			return nil, "", ErrMCPCursor
		}
		now := time.Now().Unix()
		if c.Session != scopeIdentity(scope) || c.Method != method || c.Vector != vector || c.Issued > now+5 || now-c.Issued > 600 || c.Offset < 0 || c.Offset > len(entries) {
			return nil, "", ErrMCPCursor
		}
		offset = c.Offset
	}
	end := offset + 100
	if end >= len(entries) {
		return entries[offset:], "", nil
	}
	c := mcpCursor{Session: scopeIdentity(scope), Method: method, Vector: vector, Key: w.keys.Active, Offset: end, Issued: time.Now().Unix()}
	raw, _ := json.Marshal(c)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	signature, e := w.keys.Derive("mc1", c.Key, encoded)
	if e != nil {
		return nil, "", ErrMCPUnavailable
	}
	// The signature contains periods; encode it as a single opaque cursor segment.
	return entries[offset:end], encoded + "." + base64.RawURLEncoding.EncodeToString([]byte(signature)), nil
}
func (w *MCPGateway) Call(ctx context.Context, credentials MCPCredentials, method string, raw []byte) (body []byte, handled bool, callErr error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var auditScope MCPContext
	defer func() {
		if handled {
			outcome := "success"
			if callErr != nil {
				outcome = "failed"
			}
			slog.InfoContext(ctx, "mcp_invocation", "method", method, "session_id", auditScope.SessionID, "workspace_id", auditScope.WorkspaceID, "user_id", auditScope.UserID, "turn_id", credentials.TurnID, "replica_id", credentials.ReplicaID, "execution_epoch", credentials.ExecutionEpoch, "outcome", outcome)
		}
	}()
	if listField(method) == "" && !slices.Contains([]string{"tools/call", "prompts/get", "resources/read"}, method) {
		return nil, false, nil
	}
	scope, e := w.identity.Authorize(ctx, credentials)
	if e != nil {
		return nil, true, e
	}
	auditScope = scope

	if w.business != nil && method == "tools/call" {
		var request struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return nil, true, ErrMCPArgument
		}
		if strings.HasPrefix(request.Name, w.business.DefinitionID+"__") {
			if _, err := mcpdelegation.ArgumentsDigest(raw); err != nil {
				return nil, true, ErrMCPArgument
			}
		}
	}
	var params map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&params) != nil {
		return nil, true, ErrMCPArgument
	}
	if params == nil {
		params = map[string]any{}
	}
	delete(params, "_meta")
	sources, vector, e := w.sources(ctx, scope)
	if e != nil {
		return nil, true, e
	}
	if field := listField(method); field != "" {
		entries := []map[string]any{}
		seen := map[string]bool{}
		key := "name"
		if method == "resources/list" {
			key = "uri"
		}
		if method == "resources/templates/list" {
			key = "uriTemplate"
		}
		for _, source := range sources {
			items, e := w.sourceList(ctx, source, method)
			if e != nil {
				return nil, true, e
			}
			for _, item := range items {
				name, _ := item[key].(string)
				if name == "" || seen[name] {
					return nil, true, ErrMCPUnavailable
				}
				seen[name] = true
				delete(item, "_media_origin_name")
				entries = append(entries, item)
				if len(entries) > 2000 {
					return nil, true, ErrMCPUnavailable
				}
			}
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i][key].(string) < entries[j][key].(string) })
		content, _ := json.Marshal(entries)
		hash := sha256.Sum256(append([]byte(vector), content...))
		vector = hex.EncodeToString(hash[:])
		cursor, valid := params["cursor"].(string)
		if params["cursor"] != nil && !valid {
			return nil, true, ErrMCPArgument
		}
		page, next, e := w.page(scope, method, vector, cursor, entries)
		if e != nil {
			return nil, true, e
		}
		result := map[string]any{field: page}
		if next != "" {
			result["nextCursor"] = next
		}
		body, e := json.Marshal(result)
		return body, true, e
	}
	return w.invoke(ctx, credentials, scope, sources, method, params)
}

func scopeArguments(params map[string]any, scope MCPContext) bool {
	identities := map[string]string{"workspace_id": scope.WorkspaceID, "workspaceId": scope.WorkspaceID, "session_id": scope.SessionID, "sessionId": scope.SessionID, "user_id": scope.UserID, "userId": scope.UserID, "agent_config_id": scope.AgentConfigID}
	check := func(values map[string]any) bool {
		for key, expected := range identities {
			if actual, ok := values[key]; ok && actual != expected {
				return false
			}
		}
		return true
	}
	if !check(params) {
		return false
	}
	if args, ok := params["arguments"].(map[string]any); ok {
		return check(args)
	}
	return true
}
func (w *MCPGateway) invoke(ctx context.Context, credentials MCPCredentials, scope MCPContext, sources []mcpSource, method string, params map[string]any) ([]byte, bool, error) {
	if !scopeArguments(params, scope) {
		return nil, true, ErrMCPArgument
	}
	name, _ := params["name"].(string)
	sourceID := ""
	original := ""
	if method == "resources/read" {
		uri, _ := params["uri"].(string)
		body, ok := strings.CutPrefix(uri, "mcp-gateway://")
		if !ok {
			return nil, true, ErrMCPArgument
		}
		sourceID, original, _ = strings.Cut(body, "/")
		if original == "" {
			return nil, true, ErrMCPArgument
		}
	} else {
		sourceID, _, _ = strings.Cut(name, "__")
		if name == "" {
			return nil, true, ErrMCPArgument
		}
	}
	var selected *mcpSource
	for i := range sources {
		if sources[i].definitionID == sourceID {
			selected = &sources[i]
			break
		}
	}
	if selected == nil {
		return nil, true, ErrMCPForbidden
	}
	if selected.definitionID == "platform" {
		body, e := w.platformCall(ctx, scope, method, params, original)
		return body, true, e
	}
	if !allowed(selected.grant.AllowedMethods, method) {
		return nil, true, ErrMCPForbidden
	}
	peer, e := w.upstream.Open(ctx, selected.backend)
	if e != nil {
		if errors.Is(e, client.ErrMCPReauth) {
			return nil, true, &MCPReauth{Source: sourceID}
		}
		return nil, true, ErrMCPUnavailable
	}
	defer peer.Close()
	if method == "resources/read" {
		if !allowedResource(selected.grant.AllowedResources, original) {
			return nil, true, ErrMCPForbidden
		}
		params["uri"] = original
	} else {
		list := "tools/list"
		if method == "prompts/get" {
			list = "prompts/list"
		}
		entries, e := w.sourceListPeer(ctx, *selected, list, peer)
		if e != nil {
			return nil, true, e
		}
		found := false
		for _, entry := range entries {
			if entry["name"] == name {
				original, _ = entry["_media_origin_name"].(string)
				found = true
				break
			}
		}
		if !found || original == "" {
			return nil, true, ErrMCPForbidden
		}
		params["name"] = original
	}
	// Recheck live permissions after discovery before any external invocation.
	if _, e := w.identity.Authorize(ctx, credentials); e != nil {
		return nil, true, e
	}
	currentSources, _, e := w.sources(ctx, scope)
	if e != nil {
		return nil, true, e
	}
	fresh := false
	for _, source := range currentSources {
		if source.definitionID == selected.definitionID && source.version == selected.version && allowed(source.grant.AllowedMethods, method) {
			names := source.grant.AllowedTools
			if method == "prompts/get" {
				names = source.grant.AllowedPrompts
			}
			if method == "resources/read" {
				names = source.grant.AllowedResources
			}
			authorized := allowed(names, original)
			if method == "resources/read" {
				authorized = allowedResource(names, original)
			}
			if authorized {
				*selected = source
				fresh = true
			}
			break
		}
	}
	if !fresh {
		return nil, true, ErrMCPForbidden
	}
	encoded, e := json.Marshal(params)
	if e != nil {
		return nil, true, ErrMCPArgument
	}
	raw, e := peer.Call(ctx, method, encoded)
	if errors.Is(e, client.ErrMCPReauth) {
		return nil, true, &MCPReauth{Source: sourceID}
	}
	var remote *client.MCPRemoteError
	if errors.As(e, &remote) {
		if remote.Code == -32001 {
			return nil, true, ErrMCPForbidden
		}
		return nil, true, ErrMCPArgument
	}
	if e != nil {
		return nil, true, ErrMCPOutcomeUnknown
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil {
		return nil, true, ErrMCPOutcomeUnknown
	}
	if method == "resources/read" {
		if contents, ok := result["contents"].([]any); ok {
			for _, content := range contents {
				if entry, ok := content.(map[string]any); ok {
					if uri, ok := entry["uri"].(string); ok {
						entry["uri"] = resourceURI(sourceID, uri)
					}
				}
			}
		}
	}
	rewriteResourceReferences(result, sourceID)
	body, e := json.Marshal(result)
	return body, true, e
}
func (w *MCPGateway) platformList(method string) []map[string]any {
	empty := []map[string]any{}
	switch method {
	case "tools/list":
		return []map[string]any{{"name": "platform__session_context", "description": "Read this Session's verified workspace and fixed configuration identity. Does not return credentials.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, "annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false}}, {"name": "platform__read_skill", "description": "Read the SKILL.md of a Skill selected for this Session's fixed configuration.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"skill_id": map[string]any{"type": "string"}}, "required": []string{"skill_id"}, "additionalProperties": false}, "annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false}}}
	case "prompts/list":
		return []map[string]any{{"name": "platform__platform_instructions", "description": "Platform prompt body for this Session's fixed template version."}}
	case "resources/list":
		return []map[string]any{{"name": "platform__session_context", "uri": resourceURI("platform", "media-agent://session/context"), "mimeType": "application/json"}}
	default:
		return empty
	}
}
func (w *MCPGateway) platformMetadata(ctx context.Context, scope MCPContext) (client.MetadataContext, error) {
	if w.metadata == nil {
		return client.MetadataContext{}, ErrMCPUnavailable
	}
	v, e := w.metadata.Get(ctx, scope.UserID, client.MetadataQuery{WorkspaceID: scope.WorkspaceID, AgentConfigID: scope.AgentConfigID, TargetContextVersion: scope.MetadataTargetContextVersion, ForSession: true})
	if e != nil || v.ContextVersion != scope.ContextVersion || v.WorkspaceID != scope.WorkspaceID || v.AgentConfigID != scope.AgentConfigID {
		return client.MetadataContext{}, ErrMCPUnavailable
	}
	return v, nil
}
func (w *MCPGateway) platformCall(ctx context.Context, scope MCPContext, method string, params map[string]any, uri string) ([]byte, error) {
	contextBody, _ := json.Marshal(map[string]any{"session_id": scope.SessionID, "workspace_id": scope.WorkspaceID, "agent_config_id": scope.AgentConfigID, "agent_type": scope.AgentType, "context_version": scope.ContextVersion})
	name, _ := params["name"].(string)
	switch {
	case method == "tools/call" && name == "platform__session_context":
		return json.Marshal(map[string]any{"content": []map[string]any{{"type": "text", "text": string(contextBody)}}})
	case method == "resources/read" && uri == "media-agent://session/context":
		return json.Marshal(map[string]any{"contents": []map[string]any{{"uri": resourceURI("platform", uri), "mimeType": "application/json", "text": string(contextBody)}}})
	case method == "prompts/get" && name == "platform__platform_instructions":
		v, e := w.platformMetadata(ctx, scope)
		if e != nil {
			return nil, e
		}
		body, _, e := RenderPlatformPrompt(v)
		if e != nil {
			return nil, e
		}
		return json.Marshal(map[string]any{"messages": []map[string]any{{"role": "user", "content": map[string]any{"type": "text", "text": body}}}})
	case method == "tools/call" && name == "platform__read_skill":
		return w.readSelectedSkill(ctx, scope, params)
	default:
		return nil, ErrMCPForbidden
	}
}

func (w *MCPGateway) readSelectedSkill(ctx context.Context, scope MCPContext, params map[string]any) ([]byte, error) {
	args, ok := params["arguments"].(map[string]any)
	if !ok {
		return nil, ErrMCPArgument
	}
	id, ok := args["skill_id"].(string)
	if !ok || id == "" || len(args) != 1 {
		return nil, ErrMCPArgument
	}
	metadata, e := w.platformMetadata(ctx, scope)
	if e != nil {
		return nil, e
	}
	for _, resource := range metadata.Resources {
		if resource.Kind != "media_control.skill" || resource.ID != id {
			continue
		}
		artifact, e := w.metadata.Artifact(ctx, scope.UserID, client.MetadataQuery{WorkspaceID: scope.WorkspaceID, AgentConfigID: scope.AgentConfigID, TargetContextVersion: scope.MetadataTargetContextVersion, ForSession: true}, resource.Checksum)
		if e != nil || len(artifact.Content) > 8<<20 {
			return nil, ErrMCPUnavailable
		}
		hash := sha256.Sum256(artifact.Content)
		if hex.EncodeToString(hash[:]) != resource.Checksum || artifact.Checksum != resource.Checksum {
			return nil, ErrMCPUnavailable
		}
		archive, e := zip.NewReader(bytes.NewReader(artifact.Content), int64(len(artifact.Content)))
		if e != nil {
			return nil, ErrMCPUnavailable
		}
		var body []byte
		for _, file := range archive.File {
			if file.Name != "SKILL.md" {
				continue
			}
			if body != nil || file.UncompressedSize64 > 1<<20 {
				return nil, ErrMCPUnavailable
			}
			reader, e := file.Open()
			if e != nil {
				return nil, ErrMCPUnavailable
			}
			body, e = io.ReadAll(io.LimitReader(reader, (1<<20)+1))
			reader.Close()
			if e != nil || len(body) > 1<<20 {
				return nil, ErrMCPUnavailable
			}
		}
		if body == nil {
			return nil, ErrMCPUnavailable
		}
		return json.Marshal(map[string]any{"content": []map[string]any{{"type": "text", "text": string(body)}}})
	}
	return nil, ErrMCPForbidden
}

type MCPReauth struct{ Source string }

func (e *MCPReauth) Error() string { return "MCP integration requires reconnection" }
func allowedResource(values []string, uri string) bool {
	if allowed(values, uri) {
		return true
	}
	for _, value := range values {
		if strings.Contains(value, "{") {
			tpl, e := uritemplate.New(value)
			if e == nil && tpl.Regexp().MatchString(uri) {
				return true
			}
		}
	}
	return false
}

func canonicalACL(values []string) []string {
	result := append([]string{}, values...)
	slices.Sort(result)
	return slices.Compact(result)
}
