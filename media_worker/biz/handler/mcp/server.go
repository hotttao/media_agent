package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/hertz_infra/authn/internaljwt"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/hertz_infra/mcpprotocol"
	policy "media_agent/media_worker/biz/policy/media_management"
	contract "media_agent/media_worker/biz/shared/media_management"
	flow "media_agent/media_worker/biz/workflow/media_management"
	"net/http"
	"strings"
	"time"
)

type Handler struct {
	Flow                         flow.Management
	Backend, Workflow, Materials *internaljwt.Verifier
	validators                   map[string]*mcpprotocol.Validator
}

func New(management flow.Management, backend, workflow, materials *internaljwt.Verifier) (http.Handler, error) {
	h := &Handler{Flow: management, Backend: backend, Workflow: workflow, Materials: materials, validators: map[string]*mcpprotocol.Validator{}}
	for _, name := range policy.AllNames() {
		v, err := mcpprotocol.CompileSchema(inputSchema(name))
		if err != nil {
			return nil, err
		}
		h.validators[name] = v
	}
	return mcpprotocol.NewNamedHandler("media-worker-mcp", true, h.factory), nil
}
func (h *Handler) factory(ctx context.Context, headers http.Header) (mcpprotocol.Caller, int, error) {
	raw, ok := strings.CutPrefix(headers.Get("Authorization"), "Bearer ")
	if !ok {
		return nil, 401, mcpdelegation.ErrInvalid
	}
	g, err := mcpdelegation.Verify(ctx, h.Backend, raw, mcpdelegation.Backend)
	if err != nil {
		return nil, 401, err
	}
	creds := contract.Credentials{}
	if g.Method == "tools/call" {

		audience, verifier, token := mcpdelegation.Workflow, h.Workflow, headers.Get(mcpdelegation.WorkflowHeader)
		if g.SourceID == "media-materials" {
			audience, verifier, token = mcpdelegation.Materials, h.Materials, headers.Get(mcpdelegation.MaterialsHeader)
			creds.Materials = token
		} else {
			creds.Workflow = token
		}
		target, err := mcpdelegation.Verify(ctx, verifier, token, audience)
		if err != nil || !mcpdelegation.SameInvocation(g, target) {
			return nil, 401, mcpdelegation.ErrInvalid
		}
		if g.SourceID != "media-materials" {
			if value := headers.Get(mcpdelegation.MaterialsHeader); value != "" {
				target, err := mcpdelegation.Verify(ctx, h.Materials, value, mcpdelegation.Materials)
				if err != nil || !mcpdelegation.SameInvocation(g, target) {
					return nil, 401, mcpdelegation.ErrInvalid
				}
				creds.Materials = value
			}
		}
	}
	return caller{h, g, creds}, 200, nil
}

type caller struct {
	handler     *Handler
	grant       mcpdelegation.Grant
	credentials contract.Credentials
}

func (c caller) Call(ctx context.Context, method string, raw []byte) ([]byte, bool, error) {
	if method != c.grant.Method {
		return nil, true, &mcpprotocol.RPCError{Code: -32001, Message: "Request assertion mismatch"}
	}
	if method == "tools/list" {
		entries := []any{}
		for _, name := range policy.SourceNames(c.grant.SourceID) {
			entries = append(entries, map[string]any{"name": name, "description": toolDescription(name), "inputSchema": inputSchema(name), "outputSchema": outputSchema(name), "annotations": map[string]any{"readOnlyHint": policy.ReadOnly(name), "destructiveHint": false}})
		}
		body, err := json.Marshal(map[string]any{"tools": entries})
		return body, true, err
	}
	if method != "tools/call" {
		return nil, false, nil
	}
	var request struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(raw, &request) != nil || request.Name != c.grant.Operation || !policy.InSource(c.grant.SourceID, request.Name) {
		return nil, true, invalid()
	}
	digest, err := mcpdelegation.ArgumentsDigest(request.Arguments)
	if err != nil || digest != c.grant.ArgumentsDigest {
		return nil, true, invalid()
	}
	validator := c.handler.validators[request.Name]
	if validator == nil || validator.Validate(request.Arguments) != nil {
		return nil, true, invalid()
	}
	var batch struct {
		Items []struct {
			ID     string     `json:"item_id"`
			Params parameters `json:"params"`
		} `json:"items"`
	}
	if json.Unmarshal(request.Arguments, &batch) != nil {
		return nil, true, invalid()
	}
	items := make([]flow.Item[contract.Params], len(batch.Items))
	for i, item := range batch.Items {
		items[i] = flow.Item[contract.Params]{ID: item.ID, Params: item.Params.command()}
	}
	ctx = contract.WithCredentials(mcpdelegation.WithPrincipal(ctx, c.grant.Principal), c.credentials)
	deadline := time.Now().Add(30 * time.Second)
	if c.grant.ExpiresAt.Before(deadline) {
		deadline = c.grant.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	result, err := c.handler.Flow.Execute(ctx, request.Name, items)
	if err != nil {
		if errors.Is(err, flow.ErrInvalidBatch) || errors.Is(err, policy.ErrInvalid) {
			return nil, true, invalid()
		}
		var failure *flow.Failure
		if errors.As(err, &failure) {
			return nil, true, &mcpprotocol.RPCError{Code: -32001, Message: failure.Message}
		}
		return nil, true, err
	}
	rows := make([]any, len(result.Results))
	for i, row := range result.Results {
		value := map[string]any{"item_id": row.ID, "status": row.Status}
		if row.Failure != nil {
			value["error"] = map[string]any{"code": row.Failure.Code, "message": row.Failure.Message}
		} else {
			value["data"] = row.Data
		}
		rows[i] = value
	}
	summary := result.Summary
	structured := map[string]any{"results": rows, "summary": map[string]any{"total": summary.Total, "succeeded": summary.Succeeded, "failed": summary.Failed, "unknown": summary.Unknown, "not_dispatched": summary.NotDispatched}}
	text, _ := json.Marshal(structured)
	body, err := json.Marshal(map[string]any{"structuredContent": structured, "content": []any{map[string]any{"type": "text", "text": string(text)}}, "isError": summary.Succeeded != summary.Total})
	return body, true, err
}
func invalid() error { return &mcpprotocol.RPCError{Code: -32602, Message: "Invalid batch arguments"} }

func toolDescription(name string) string {
	description := name + ": batch operation. Pass items with unique item_id and typed params; each result is independent. Never retry the whole batch after partial failure."
	switch name {
	case "jobs_submit":
		description += " Submit independent jobs asynchronously. Each successful item returns job_id and current status immediately after acceptance; generation continues in the background. Use jobs_get with the returned IDs to check progress and results. Keep each item's idempotency_key when retrying that item."
	case "jobs_get":
		description += " Query current job status and available artifacts by the IDs returned from jobs_submit. This does not wait for completion or resubmit jobs."
	}
	return description
}
