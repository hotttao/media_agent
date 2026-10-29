package media_management

import (
	"context"
	"encoding/json"
	"errors"
	"media_agent/hertz_infra/authn/mcpdelegation"
	policy "media_agent/media_worker/biz/policy/media_management"
	contract "media_agent/media_worker/biz/shared/media_management"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type Objects interface {
	Get(context.Context, string, string) (map[string]any, error)
	Resolve(context.Context, string) (map[string]any, error)
}

// Operations coordinates read prerequisites and resource resolution before a
// single business mutation. Only the resource API commits business facts.
type Operations struct {
	API     contract.API
	Objects Objects
}

func (o Operations) Execute(ctx context.Context, name string, p contract.Params) (map[string]any, error) {
	if o.API == nil {
		return nil, &Failure{Code: "unavailable", Message: "Business API unavailable", NotDispatched: true}
	}
	switch name {
	case "templates_create":
		capability, err := o.API.Execute(ctx, "capabilities_get", contract.Params{CapabilityKey: p.CapabilityKey, CapabilityVersion: p.CapabilityVersion})
		if err != nil {
			return nil, err
		}
		if textValue(capability, "status") != "published" {
			return nil, &Failure{Code: "capability_unavailable", Message: "Capability must be published", NotDispatched: true}
		}
		p.Provider = textValue(capability, "provider")
		p.GenerationMethod = textValue(capability, "generation_method")
		p.GenerationType = textValue(capability, "generation_type")
	case "templates_publish":
		template, err := o.API.Execute(ctx, "templates_get", contract.Params{ID: p.ID})
		if err != nil {
			return nil, err
		}
		if err = allowedTemplate(template); err != nil {
			return nil, err
		}
	case "workflows_create", "workflows_validate":
		for _, step := range p.Steps {
			template, err := o.API.Execute(ctx, "templates_get", contract.Params{ID: step.TemplateID, Version: step.TemplateVersion})
			if err != nil {
				return nil, err
			}
			if err = allowedTemplate(template); err != nil {
				return nil, err
			}
			bindings := map[string]any{}
			for slot, b := range step.Bindings {
				bindings[slot] = map[string]any{"source": b.Source, "ref": b.Ref, "const_value": b.ConstValue}
			}
			if err = validateImageBindings(template, bindings, nil); err != nil {
				return nil, err
			}
		}
	case "workflows_publish", "jobs_submit":
		id := p.ID
		if name == "jobs_submit" {
			id = p.WorkflowID
		}
		workflow, err := o.API.Execute(ctx, "workflows_get", contract.Params{ID: id})
		if err != nil {
			return nil, err
		}
		if name == "jobs_submit" && numberValue(workflow, "version") != p.WorkflowVersion {
			return nil, &Failure{Code: "version_conflict", Message: "Workflow version changed", NotDispatched: true}
		}
		steps, ok := workflow["steps"].([]any)
		if !ok || len(steps) == 0 {
			return nil, &Failure{Code: "invalid_workflow", Message: "Workflow has no executable steps", NotDispatched: true}
		}
		for _, raw := range steps {
			s, ok := raw.(map[string]any)
			if !ok {
				return nil, &Failure{Code: "invalid_workflow", Message: "Invalid workflow definition", NotDispatched: true}
			}
			template, err := o.API.Execute(ctx, "templates_get", contract.Params{ID: textValue(s, "template_id"), Version: numberValue(s, "template_version")})
			if err != nil {
				return nil, err
			}
			if err = allowedTemplate(template); err != nil {
				return nil, err
			}
			bindings, _ := s["slot_bindings"].(map[string]any)
			var input *contract.JobInput
			if name == "jobs_submit" {
				input = &p.Input
			}
			if err = validateImageBindings(template, bindings, input); err != nil {
				return nil, err
			}
		}
		if name == "jobs_submit" {
			input, err := o.resolveInput(ctx, p.Input, textValue(workflow, "scope"))
			if err != nil {
				return nil, err
			}
			p.Input = input
		}
	case "capabilities_list":
		return o.capabilities(ctx, p)
	}
	result, err := o.API.Execute(ctx, name, p)
	if err != nil {
		return nil, err
	}
	if name == "jobs_submit" {
		if textValue(result, "job_id") == "" || result["status"] == nil {
			return nil, &Failure{Code: "outcome_unknown", Message: "Submission receipt did not include a Job ID and status", Unknown: true}
		}
		return selectFields(result, "job_id", "status"), nil
	}
	if name == "jobs_get" {
		o.artifacts(ctx, result)
	}
	return result, nil
}
func allowedTemplate(value map[string]any) error {
	if !policy.Supported(textValue(value, "capability_key"), numberValue(value, "capability_version")) {
		return &Failure{Code: "unsupported_capability", Message: "Only published Doubao video and GPT image capabilities are supported", NotDispatched: true}
	}
	return nil
}
func textValue(v map[string]any, key string) string { s, _ := v[key].(string); return s }
func numberValue(v map[string]any, key string) int {
	switch n := v[key].(type) {
	case json.Number:
		i, _ := strconv.Atoi(string(n))
		return i
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
func (o Operations) resolveInput(ctx context.Context, input contract.JobInput, scope string) (contract.JobInput, error) {
	needs := input.ProductID != "" || input.VirtualIPID != ""
	for _, items := range [][]contract.MediaInput{input.Assets, input.Materials} {
		for _, i := range items {
			needs = needs || i.ObjectPath != ""
		}
	}
	if needs && o.Objects == nil {
		return input, &Failure{Code: "materials_unavailable", Message: "Object service unavailable", NotDispatched: true}
	}
	if input.ProductID != "" {
		v, err := o.Objects.Get(ctx, "products", input.ProductID)
		if err != nil {
			return input, err
		}
		if scope == "" || textValue(v, "scope") != scope {
			return input, &Failure{Code: "scope_mismatch", Message: "Product and workflow must use the same business scope", NotDispatched: true}
		}
		input.Product = selectFields(v, "id", "name", "target_audience", "product_details", "display_actions")
		images, _ := v["images"].([]any)
		path := ""
		for _, raw := range images {
			image, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			candidate := textValue(image, "object_path")
			if candidate == "" {
				continue
			}
			if path == "" {
				path = candidate
			}
			if main, _ := image["is_main"].(bool); main {
				path = candidate
				break
			}
		}
		if path != "" {
			url, err := o.imageURL(ctx, path)
			if err != nil {
				return input, err
			}
			input.Product["image_url"] = url
		}
	}
	if input.VirtualIPID != "" {
		v, err := o.Objects.Get(ctx, "virtual-ips", input.VirtualIPID)
		if err != nil {
			return input, err
		}
		if scope == "" || textValue(v, "scope") != scope {
			return input, &Failure{Code: "scope_mismatch", Message: "Virtual IP and workflow must use the same business scope", NotDispatched: true}
		}
		input.VirtualIP = selectFields(v, "id", "name")
		for _, field := range []string{"avatar", "full_body", "three_view", "nine_view"} {
			if path := textValue(v, field+"_object_path"); path != "" {
				url, err := o.imageURL(ctx, path)
				if err != nil {
					return input, err
				}
				input.VirtualIP[field+"_url"] = url
			}
		}
	}
	resolve := func(items []contract.MediaInput) ([]contract.MediaInput, error) {
		out := append([]contract.MediaInput(nil), items...)
		for i, item := range out {
			if item.ObjectPath != "" && item.Content != "" {
				return nil, &Failure{Code: "invalid_argument", Message: "Choose object_path or content", NotDispatched: true}
			}
			if item.ObjectPath == "" {
				continue
			}
			if strings.Contains(item.ObjectPath, "://") || strings.HasPrefix(item.ObjectPath, "/") || strings.Contains(item.ObjectPath, "..") || strings.ContainsAny(item.ObjectPath, "\\\x00") {
				return nil, &Failure{Code: "invalid_argument", Message: "Expected a platform object path", NotDispatched: true}
			}
			value, err := o.Objects.Resolve(ctx, item.ObjectPath)
			if err != nil {
				return nil, err
			}
			out[i].URL = textValue(value, "url")
			if !strings.HasPrefix(textValue(value, "content_type"), "image/") {
				return nil, &Failure{Code: "invalid_object_type", Message: "Supported generation inputs require image objects", NotDispatched: true}
			}
			if out[i].URL == "" {
				return nil, &Failure{Code: "object_unavailable", Message: "Object has no usable URL", NotDispatched: true}
			}
		}
		return out, nil
	}
	var err error
	input.Assets, err = resolve(input.Assets)
	if err != nil {
		return input, err
	}
	input.Materials, err = resolve(input.Materials)
	return input, err
}
func selectFields(value map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, key := range keys {
		if v, ok := value[key]; ok {
			out[key] = v
		}
	}
	return out
}
func (o Operations) capabilities(ctx context.Context, p contract.Params) (map[string]any, error) {
	found := []any{}
	for _, key := range []string{"doubao.i2v.first_frame_prompt", "gpt.t2i.text_prompt", "gpt.i2i.multi_image_prompt"} {
		value, err := o.API.Execute(ctx, "capabilities_get", contract.Params{CapabilityKey: key, CapabilityVersion: 1})
		if err != nil {
			var failure *Failure
			if errors.As(err, &failure) && failure.Code == "not_found_or_forbidden" {
				continue
			}
			return nil, err
		}
		if textValue(value, "status") != "published" || p.Provider != "" && textValue(value, "provider") != p.Provider || p.GenerationMethod != "" && textValue(value, "generation_method") != p.GenerationMethod || p.GenerationType != "" && textValue(value, "generation_type") != p.GenerationType {
			continue
		}
		found = append(found, value)
	}
	start := min((p.Page-1)*p.PageSize, len(found))
	end := min(start+p.PageSize, len(found))
	return map[string]any{"items": found[start:end], "total": len(found), "page": p.Page, "page_size": p.PageSize}, nil
}
func (o Operations) artifacts(ctx context.Context, result map[string]any) {
	steps, _ := result["step_runs"].([]any)
	for _, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		// Never leak old candidate signed URLs even when artifact enrichment fails.
		candidates, _ := step["candidates"].([]any)
		for _, raw := range candidates {
			candidate, ok := raw.(map[string]any)
			if ok && textValue(candidate, "url") != "" {
				o.candidateArtifact(ctx, candidate)
			}
		}
		id := textValue(step, "output_asset_id")
		if id == "" {
			continue
		}
		step["artifact_access"] = "unavailable"
		if o.Objects == nil {
			continue
		}
		asset, err := o.Objects.Get(ctx, "generated-assets", id)
		if err != nil {
			step["artifact_access"] = artifactError(err)
			continue
		}
		path := textValue(asset, "object_path")
		if path == "" {
			continue
		}
		resolved, err := o.Objects.Resolve(ctx, path)
		if err != nil {
			step["artifact_access"] = artifactError(err)
			continue
		}
		step["artifact_access"] = "available"
		step["artifact"] = selectFields(resolved, "url", "expires_at", "content_type", "size_bytes", "checksum_sha256")
	}
}
func artifactError(err error) string {
	if failure, ok := err.(*Failure); ok && (failure.Code == "permission_denied" || failure.Code == "not_found_or_forbidden") {
		return "denied"
	}
	return "unavailable"
}

var _ contract.API = Operations{}

func (o Operations) imageURL(ctx context.Context, path string) (string, error) {
	resolved, err := o.Objects.Resolve(ctx, path)
	if err != nil {
		return "", err
	}
	url := textValue(resolved, "url")
	if url == "" || !strings.HasPrefix(textValue(resolved, "content_type"), "image/") {
		return "", &Failure{Code: "invalid_object_type", Message: "Referenced object must be an image", NotDispatched: true}
	}
	return url, nil
}

// Candidate links belong to the already-authorized Job. Only their object path
// is reused; the original URL is never fetched or returned.
func (o Operations) candidateArtifact(ctx context.Context, candidate map[string]any) {
	raw := textValue(candidate, "url")
	delete(candidate, "url")
	candidate["artifact_access"] = "unavailable"
	principal, ok := mcpdelegation.FromContext(ctx)
	if !ok || o.Objects == nil {
		return
	}
	prefix := "materials/" + principal.OrganizationID + "/"
	objectPath := strings.TrimPrefix(raw, "object_path:")
	if parsed, err := url.Parse(raw); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
		marker := "/" + prefix
		at := strings.Index(parsed.Path, marker)
		if at < 0 {
			return
		}
		objectPath = parsed.Path[at+1:]
	}
	if !strings.HasPrefix(objectPath, prefix) || path.Clean(objectPath) != objectPath || strings.ContainsAny(objectPath, "\\\x00") {
		return
	}
	resolved, err := o.Objects.Resolve(ctx, objectPath)
	if err != nil {
		candidate["artifact_access"] = artifactError(err)
		return
	}
	if textValue(resolved, "url") == "" {
		return
	}
	candidate["artifact_access"] = "available"
	candidate["artifact"] = selectFields(resolved, "url", "expires_at", "content_type", "size_bytes", "checksum_sha256")
}

// Current executable capabilities take image resource references. Their
// asset_ref expressions identify slots which must never consume text/URLs
// disguised as a constant or a media content field.
func validateImageBindings(template, bindings map[string]any, input *contract.JobInput) error {
	slots := map[string]bool{}
	var visit func(any)
	visit = func(raw any) {
		switch expression := raw.(type) {
		case map[string]any:
			if textValue(expression, "op") == "asset_ref" {
				slots[strings.Split(textValue(expression, "slot"), ".")[0]] = true
			}
			for _, value := range expression {
				visit(value)
			}
		case []any:
			for _, value := range expression {
				visit(value)
			}
		}
	}
	visit(template["invocation_mapping"])
	for slot := range slots {
		binding, _ := bindings[slot].(map[string]any)
		source := numberValue(binding, "source")
		sourceName := textValue(binding, "source")
		if source == 3 || sourceName == "CONST" {
			return &Failure{Code: "invalid_argument", Message: "Image slots must reference workflow inputs or step outputs", NotDispatched: true}
		}
		if input == nil || source != 1 && sourceName != "WORKFLOW_INPUT" {
			continue
		}
		ref := textValue(binding, "ref")
		for _, items := range [][]contract.MediaInput{input.Assets, input.Materials} {
			for _, item := range items {
				if item.Name == ref && (item.ObjectPath == "" || item.Content != "") {
					return &Failure{Code: "invalid_argument", Message: "Image input requires an object_path and cannot use content", NotDispatched: true}
				}
			}
		}
	}
	return nil
}
