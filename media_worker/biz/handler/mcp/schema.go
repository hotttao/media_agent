package mcp

import "strings"

type schema = map[string]any

func object(properties schema, required ...string) schema {
	return schema{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func text() schema              { return schema{"type": "string"} }
func nonempty() schema          { return schema{"type": "string", "minLength": 1} }
func integer(min int) schema    { return schema{"type": "integer", "minimum": min} }
func array(items schema) schema { return schema{"type": "array", "items": items} }
func paramsSchema(name string) schema {
	p := schema{}
	required := []string{}
	if strings.HasSuffix(name, "_list") {
		p["page"] = schema{"type": "integer", "minimum": 1, "maximum": 2147483647}
		p["page_size"] = schema{"type": "integer", "minimum": 1, "maximum": 100}
		if name == "capabilities_list" {
			for _, key := range []string{"provider", "generation_method", "generation_type"} {
				p[key] = text()
			}
		} else {
			p["scope"] = text()
			if name == "jobs_list" {
				p["workflow_id"] = text()
				p["status"] = text()
			} else {
				p["name"] = text()
				p["visibility"] = text()
				if name == "workflows_list" {
					p["status"] = text()
				}
			}
		}
	}
	if strings.HasSuffix(name, "_get") || strings.HasSuffix(name, "_publish") {
		p["id"] = nonempty()
		required = append(required, "id")
		if name == "templates_get" || name == "workflows_get" {
			p["version"] = integer(1)
		}
	}
	if name == "templates_create" {
		for _, key := range []string{"scope", "name", "prompt_body", "output_asset_kind", "capability_key"} {
			p[key] = nonempty()
			required = append(required, key)
		}
		p["description"] = text()
		p["capability_version"] = integer(1)
		required = append(required, "capability_version", "inputs", "invocation_mapping")
		p["needs_confirm"] = schema{"type": "boolean"}
		p["candidate_count"] = integer(0)
		p["inputs"] = array(object(schema{"name": nonempty(), "resource_type": schema{"type": "string", "enum": []string{"MATERIAL", "ASSET", "PRODUCT", "VIRTUAL_IP", "TEXT"}}, "asset_kind": text(), "required": schema{"type": "boolean"}, "display_order": integer(0)}, "name", "resource_type"))
		p["invocation_mapping"] = schema{"type": "object", "additionalProperties": schema{"$ref": "#/$defs/expression"}}
	}
	if name == "workflows_create" || name == "workflows_validate" {
		p["scope"] = nonempty()
		p["output_step_key"] = nonempty()
		required = append(required, "scope", "steps", "output_step_key")
		if name == "workflows_create" {
			p["name"] = nonempty()
			p["description"] = text()
			required = append(required, "name")
		}
		binding := object(schema{"source": schema{"type": "integer", "enum": []int{1, 2, 3}}, "ref": text(), "const_value": text()}, "source")
		step := object(schema{"key": nonempty(), "name": text(), "template_id": nonempty(), "template_version": integer(1), "step_order": integer(0), "approval_policy": schema{"type": "string", "enum": []string{"always", "when_multiple_candidates"}}, "reuse_policy": schema{"type": "string", "enum": []string{"reuse_if_available", "force_new"}}, "slot_bindings": schema{"type": "object", "additionalProperties": binding}}, "key", "template_id", "template_version", "slot_bindings")
		p["steps"] = schema{"type": "array", "minItems": 1, "maxItems": 100, "items": step}
	}
	if name == "jobs_submit" {
		p["workflow_id"] = nonempty()
		p["workflow_version"] = integer(1)
		p["idempotency_key"] = schema{"type": "string", "minLength": 1, "maxLength": 160}
		media := object(schema{"name": nonempty(), "asset_kind": nonempty(), "object_path": text(), "content": text()}, "name", "asset_kind")
		p["input"] = object(schema{"product_id": nonempty(), "virtual_ip_id": nonempty(), "asset": array(media), "material": array(media)})
		required = []string{"workflow_id", "workflow_version", "idempotency_key", "input"}
	}
	return object(p, required...)
}
func inputSchema(name string) schema {
	item := object(schema{"item_id": schema{"type": "string", "minLength": 1, "maxLength": 64}, "params": paramsSchema(name)}, "item_id", "params")
	out := object(schema{"items": schema{"type": "array", "minItems": 1, "maxItems": 20, "items": item}}, "items")
	out["$defs"] = schema{"expression": object(schema{"op": schema{"type": "string", "enum": []string{"render_template", "asset_ref", "list"}}, "slot": text(), "items": array(schema{"$ref": "#/$defs/expression"})}, "op")}
	return out
}
func outputSchema(name string) schema {
	data := schema{"type": "object"}
	if name == "jobs_submit" {
		// The native protobuf HTTP API can encode status as an enum number or name.
		data = object(schema{"job_id": nonempty(), "status": schema{"oneOf": []any{nonempty(), integer(0)}}}, "job_id", "status")
	}
	success := object(schema{"item_id": nonempty(), "status": schema{"const": "succeeded"}, "data": data}, "item_id", "status", "data")
	failure := object(schema{"item_id": nonempty(), "status": schema{"enum": []string{"failed", "outcome_unknown", "not_dispatched"}}, "error": object(schema{"code": nonempty(), "message": text()}, "code", "message")}, "item_id", "status", "error")
	result := schema{"oneOf": []any{success, failure}}

	summary := schema{}
	for _, key := range []string{"total", "succeeded", "failed", "unknown", "not_dispatched"} {
		summary[key] = integer(0)
	}
	return object(schema{"results": array(result), "summary": object(summary, "total", "succeeded", "failed", "unknown", "not_dispatched")}, "results", "summary")
}
