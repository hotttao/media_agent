package materials_api

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

func (c Client) Resolve(ctx context.Context, path string) (map[string]any, error) {
	p, ok := mcpdelegation.FromContext(ctx)
	if !ok {
		return nil, mcpdelegation.ErrInvalid
	}
	return c.HTTP.Request(ctx, "GET", "/api/v1/library-objects:resolve", url.Values{"organization_id": {p.OrganizationID}, "object_path": {path}}, nil)
}
func (c Client) Get(ctx context.Context, kind, id string) (map[string]any, error) {
	if kind != "materials" && kind != "products" && kind != "virtual-ips" && kind != "generated-assets" {
		return nil, &contract.Failure{Code: "invalid_argument", Message: "Invalid resource kind", NotDispatched: true}
	}
	p, ok := mcpdelegation.FromContext(ctx)
	if !ok {
		return nil, mcpdelegation.ErrInvalid
	}
	return c.HTTP.Request(ctx, "GET", "/api/v1/"+kind+"/"+url.PathEscape(id), url.Values{"organization_id": {p.OrganizationID}}, nil)
}

func (c Client) Execute(ctx context.Context, operation string, params contract.Params) (map[string]any, error) {
	resources := map[string]string{"virtual_ips_list": "virtual-ips", "virtual_ips_get": "virtual-ips", "materials_list": "materials", "materials_get": "materials", "generated_assets_list": "generated-assets", "generated_assets_get": "generated-assets"}
	kind, ok := resources[operation]
	if !ok {
		return nil, &contract.Failure{Code: "invalid_argument", Message: "Unknown material query", NotDispatched: true}
	}
	if strings.HasSuffix(operation, "_get") {
		return c.Get(ctx, kind, params.ID)
	}
	principal, ok := mcpdelegation.FromContext(ctx)
	if !ok {
		return nil, mcpdelegation.ErrInvalid
	}
	q := url.Values{"organization_id": {principal.OrganizationID}, "page": {strconv.Itoa(params.Page)}, "page_size": {strconv.Itoa(params.PageSize)}}
	fields := map[string]string{"scope": params.Scope}
	switch operation {
	case "virtual_ips_list":
		fields["name"] = params.Name
		fields["status"] = params.Status
		fields["visibility"] = params.Visibility
	case "materials_list":
		fields["asset_kind"] = params.AssetKind
		fields["visibility"] = params.Visibility
		if params.OnlyMine != nil {
			q.Set("only_mine", strconv.FormatBool(*params.OnlyMine))
		}
	case "generated_assets_list":
		fields["asset_kind"] = params.AssetKind
		fields["job_id"] = params.JobID
		fields["source_virtual_ip_id"] = params.SourceVirtualIPID
		fields["source_product_id"] = params.SourceProductID
	}
	for key, value := range fields {
		if value != "" {
			q.Set(key, value)
		}
	}
	return c.HTTP.Request(ctx, "GET", "/api/v1/"+kind, q, nil)
}
