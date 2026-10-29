package materials_api

import (
	"context"
	"media_agent/hertz_infra/authn/mcpdelegation"
	"media_agent/media_worker/biz/client/business_http"
	contract "media_agent/media_worker/biz/shared/media_management"
	"net/url"
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
	if kind != "products" && kind != "virtual-ips" && kind != "generated-assets" {
		return nil, &contract.Failure{Code: "invalid_argument", Message: "Invalid resource kind", NotDispatched: true}
	}
	p, ok := mcpdelegation.FromContext(ctx)
	if !ok {
		return nil, mcpdelegation.ErrInvalid
	}
	return c.HTTP.Request(ctx, "GET", "/api/v1/"+kind+"/"+url.PathEscape(id), url.Values{"organization_id": {p.OrganizationID}}, nil)
}
