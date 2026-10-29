package workflow_api

import (
	"context"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/media_worker/biz/client/business_http"
	contract "media_agent/media_worker/biz/shared/media_management"
	"net/url"
	"strconv"
	"strings"
)

type Client struct{ HTTP *business_http.Client }

func (c Client) Execute(ctx context.Context, operation string, p contract.Params) (map[string]any, error) {
	principal, ok := mcpdelegation.FromContext(ctx)
	if !ok {
		return nil, mcpdelegation.ErrInvalid
	}
	method, path := "GET", ""
	q := url.Values{}
	var body any
	switch operation {
	case "templates_list", "workflows_list", "jobs_list", "capabilities_list":
		path = "/api/v1/" + strings.TrimSuffix(operation, "_list")
		q.Set("page", strconv.Itoa(p.Page))
		q.Set("page_size", strconv.Itoa(p.PageSize))
		for k, v := range map[string]string{"scope": p.Scope, "name": p.Name, "visibility": p.Visibility, "status": p.Status, "workflow_id": p.WorkflowID, "provider": p.Provider, "generation_method": p.GenerationMethod, "generation_type": p.GenerationType} {
			if v != "" {
				q.Set(k, v)
			}
		}
	case "templates_get", "workflows_get", "jobs_get":
		path = "/api/v1/" + strings.TrimSuffix(operation, "_get") + "/" + url.PathEscape(p.ID)
		if p.Version > 0 {
			path += "/versions/" + strconv.Itoa(p.Version)
		}
	case "capabilities_get":
		path = "/api/v1/capabilities/" + url.PathEscape(p.CapabilityKey) + "/versions/" + strconv.Itoa(p.CapabilityVersion)
	case "templates_publish", "workflows_publish":
		method = "POST"
		path = "/api/v1/" + strings.TrimSuffix(operation, "_publish") + "/" + url.PathEscape(p.ID) + "/publish"
	case "templates_create":
		method = "POST"
		path = "/api/v1/templates"
		q.Set("scope", p.Scope)
		slots := []any{}
		for _, s := range p.Inputs {
			slots = append(slots, map[string]any{"name": s.Name, "resource_type": s.ResourceType, "required": s.Required, "display_order": s.DisplayOrder, "asset_kind": s.AssetKind})
		}
		mapping := map[string]any{}
		for k, v := range p.InvocationMapping {
			mapping[k] = expression(v)
		}
		body = map[string]any{"organization_id": principal.OrganizationID, "name": p.Name, "description": p.Description, "prompt_body": p.PromptBody, "inputs": slots, "output_asset_kind": p.OutputAssetKind, "capability_key": p.CapabilityKey, "capability_version": p.CapabilityVersion, "invocation_mapping": mapping, "needs_confirm": p.NeedsConfirm, "candidate_count": p.CandidateCount, "platform": p.Provider, "gen_way": p.GenerationMethod, "gen_type": p.GenerationType}
	case "workflows_create", "workflows_validate":
		method = "POST"
		path = "/api/v1/workflows"
		steps := []any{}
		for _, s := range p.Steps {
			bindings := map[string]any{}
			for k, b := range s.Bindings {
				bindings[k] = map[string]any{"source": b.Source, "ref": b.Ref, "const_value": b.ConstValue}
			}
			steps = append(steps, map[string]any{"key": s.Key, "name": s.Name, "template_id": s.TemplateID, "template_version": s.TemplateVersion, "step_order": s.Order, "slot_bindings": bindings, "approval_policy": s.ApprovalPolicy, "reuse_policy": s.ReusePolicy})
		}
		value := map[string]any{"scope": p.Scope, "steps": steps, "output_step_key": p.OutputStepKey}
		if operation == "workflows_validate" {
			path += "/validate"
		} else {
			q.Set("scope", p.Scope)
			value["organization_id"] = principal.OrganizationID
			value["workspace_id"] = principal.WorkspaceID
			value["name"] = p.Name
			value["description"] = p.Description
		}
		body = value
	case "jobs_submit":
		method = "POST"
		path = "/api/v1/jobs"
		q.Set("workflow_id", p.WorkflowID)
		body = map[string]any{"workflow_version": p.WorkflowVersion, "organization_id": principal.OrganizationID, "workspace_id": principal.WorkspaceID, "idempotency_key": p.IdempotencyKey, "input": inputBody(p.Input)}
	default:
		return nil, &contract.Failure{Code: "invalid_argument", Message: "Unknown business operation", NotDispatched: true}
	}
	return c.HTTP.Request(ctx, method, path, q, body)
}
func expression(e contract.Expression) map[string]any {
	v := map[string]any{"op": e.Op}
	if e.Slot != "" {
		v["slot"] = e.Slot
	}
	if len(e.Items) > 0 {
		items := []any{}
		for _, i := range e.Items {
			items = append(items, expression(i))
		}
		v["items"] = items
	}
	return v
}
func inputBody(i contract.JobInput) map[string]any {
	v := map[string]any{}
	if i.Product != nil {
		v["product"] = i.Product
	}
	if i.VirtualIP != nil {
		v["virtual_ip"] = i.VirtualIP
	}
	for key, items := range map[string][]contract.MediaInput{"asset": i.Assets, "material": i.Materials} {
		rows := []any{}
		for _, item := range items {
			rows = append(rows, map[string]any{"name": item.Name, "asset_kind": item.AssetKind, "url": item.URL, "content": item.Content})
		}
		v[key] = rows
	}
	return v
}
