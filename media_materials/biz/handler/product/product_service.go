package product

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_materials/product"
	domain "media_agent/media_materials/biz/domain/product"
	productsvc "media_agent/media_materials/biz/service/product"
	"media_agent/media_materials/biz/shared/auth"
	"media_agent/media_materials/biz/shared/httperr"
)

var Svc *productsvc.Service

func CreateProduct(ctx context.Context, c *app.RequestContext) {
	var req pb.CreateProductReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	p, err := Svc.Create(ctx, productsvc.CreateCommand{OrganizationID: req.GetOrganizationId(), Scope: req.GetScope(), Name: req.GetName(),
		Description: req.GetDescription(), TargetAudience: req.GetTargetAudience(), ProductDetails: req.GetProductDetails(),
		DisplayActions: req.GetDisplayActions(), Tags: req.GetTags(), ExternalCommerce: req.GetExternalCommerce(), Metadata: req.GetMetadata(),
		Images: imageCommands(req.GetImages()), CreatedBy: auth.CallerID(ctx)})
	if err != nil {
		httperr.Write(c, err, 400, "product")
		return
	}
	c.JSON(201, toView(p))
}
func GetProduct(ctx context.Context, c *app.RequestContext) {
	var req pb.GetProductReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	p, err := Svc.Get(ctx, req.GetId())
	if err != nil || p.OrganizationID != req.GetOrganizationId() {
		c.String(404, "product not found")
		return
	}
	c.JSON(200, toView(p))
}
func ListProducts(ctx context.Context, c *app.RequestContext) {
	var req pb.ListProductsReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	page, size := int(req.GetPage()), int(req.GetPageSize())
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 50
	}
	items, total, err := Svc.List(ctx, productsvc.ListQuery{OrganizationID: req.GetOrganizationId(), Scope: req.GetScope(), Name: req.GetName(),
		TargetAudience: req.GetTargetAudience(), Tag: req.GetTag(), Status: req.GetStatus(), Limit: size, Offset: (page - 1) * size})
	if err != nil {
		httperr.Write(c, err, 500, "product")
		return
	}
	rows := make([]*pb.Product, len(items))
	for i := range items {
		rows[i] = toView(items[i])
	}
	c.JSON(200, &pb.ListProductsResp{Items: rows, Total: int32(total)})
}
func UpdateProduct(ctx context.Context, c *app.RequestContext) {
	var req pb.UpdateProductReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	p, err := Svc.Update(ctx, productsvc.UpdateCommand{ID: req.GetId(), OrganizationID: req.GetOrganizationId(), Name: req.GetName(),
		Description: req.GetDescription(), TargetAudience: req.GetTargetAudience(), ProductDetails: req.GetProductDetails(),
		DisplayActions: req.GetDisplayActions(), Tags: req.GetTags(), ExternalCommerce: req.GetExternalCommerce(), Metadata: req.GetMetadata(),
		Images: imageCommands(req.GetImages()), UpdatedBy: auth.CallerID(ctx)})
	if err != nil {
		httperr.Write(c, err, 400, "product")
		return
	}
	c.JSON(200, toView(p))
}
func ArchiveProduct(ctx context.Context, c *app.RequestContext) {
	var req pb.GetProductReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	p, err := Svc.Archive(ctx, req.GetId(), req.GetOrganizationId())
	if err != nil {
		httperr.Write(c, err, 400, "product")
		return
	}
	c.JSON(200, toView(p))
}
func DeleteProduct(ctx context.Context, c *app.RequestContext) {
	var req pb.GetProductReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	p, err := Svc.Delete(ctx, req.GetId(), req.GetOrganizationId())
	if err != nil {
		httperr.Write(c, err, 400, "product")
		return
	}
	c.JSON(200, toView(p))
}
func imageCommands(rows []*pb.ProductImageInput) []productsvc.ImageCommand {
	out := make([]productsvc.ImageCommand, len(rows))
	for i, row := range rows {
		out[i] = productsvc.ImageCommand{ObjectPath: row.GetObjectPath(), IsMain: row.GetIsMain(), SortOrder: int(row.GetSortOrder())}
	}
	return out
}
func toView(p *domain.Product) *pb.Product {
	images := make([]*pb.ProductImage, len(p.Images))
	for i, image := range p.Images {
		images[i] = &pb.ProductImage{Id: image.ID, ObjectPath: image.ObjectPath, IsMain: image.IsMain, SortOrder: int32(image.SortOrder)}
	}
	return &pb.Product{Id: p.ID, OrganizationId: p.OrganizationID, Scope: p.Scope, Name: p.Name, Description: p.Description,
		TargetAudience: p.TargetAudience, ProductDetails: p.ProductDetails, DisplayActions: p.DisplayActions, Tags: p.Tags,
		ExternalCommerce: p.ExternalCommerce, Metadata: p.Metadata, Images: images, Status: string(p.Status), CreatedBy: p.CreatedBy,
		UpdatedBy: p.UpdatedBy, CreatedAt: p.CreatedAt.UnixNano(), UpdatedAt: p.UpdatedAt.UnixNano()}
}

func requireOrganization(ctx context.Context, c *app.RequestContext, id string) bool {
	if auth.HasOrganization(ctx, id) {
		return true
	}
	c.String(404, "organization not found")
	return false
}
