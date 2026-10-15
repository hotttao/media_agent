package workflow

import (
	"context"
	"media_agent/hertz_infra/authn/resourceaccess"
	d "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
	service "media_agent/media_agent_gateway/biz/service/mcp_catalog"
	"media_agent/media_agent_gateway/biz/shared/client"
)

type CatalogIdentityFacts interface {
	Resolve(context.Context, string, string) (resourceaccess.Facts, error)
}

// MCPCatalogManagement obtains current enterprise facts before catalog operations.
// Platform roles authorize only platform definitions, never enterprise administration.
type MCPCatalogManagement struct {
	Catalog *service.Service
	Facts   CatalogIdentityFacts
}

func (w *MCPCatalogManagement) authorize(ctx context.Context, user client.GatewayUser, organization string, write bool) error {
	if user.UserID == "" {
		return d.ErrForbidden
	}
	if organization == "" {
		if write && user.Role != "admin" {
			return d.ErrForbidden
		}
		return nil
	}
	if w.Facts == nil {
		return ErrMCPUnavailable
	}
	facts, err := w.Facts.Resolve(ctx, user.UserID, organization)
	if err != nil {
		return err
	}
	if !facts.Member || write && facts.OrganizationRole != "admin" {
		return d.ErrForbidden
	}
	return nil
}
func (w *MCPCatalogManagement) Put(ctx context.Context, user client.GatewayUser, value d.Definition, expected int64) (*d.Definition, error) {
	if err := w.authorize(ctx, user, value.OrganizationID, true); err != nil {
		return nil, err
	}
	return w.Catalog.PutScoped(ctx, value, expected, value.OrganizationID)
}
func (w *MCPCatalogManagement) Get(ctx context.Context, user client.GatewayUser, id string) (*d.Definition, error) {
	value, err := w.Catalog.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err = w.authorize(ctx, user, value.OrganizationID, false); err != nil {
		return nil, err
	}
	return value, nil
}
func (w *MCPCatalogManagement) List(ctx context.Context, user client.GatewayUser, organization, after string, limit int) ([]d.Definition, error) {
	if err := w.authorize(ctx, user, organization, false); err != nil {
		return nil, err
	}
	return w.Catalog.ListVisible(ctx, organization, after, limit)
}
