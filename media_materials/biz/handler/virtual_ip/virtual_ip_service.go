package virtual_ip

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_materials/virtual_ip"
	domain "media_agent/media_materials/biz/domain/virtual_ip"
	vipsvc "media_agent/media_materials/biz/service/virtual_ip"
	"media_agent/media_materials/biz/shared/auth"
	"media_agent/media_materials/biz/shared/httperr"
)

var Svc *vipsvc.Service

func CreateVirtualIp(ctx context.Context, c *app.RequestContext) {
	var req pb.CreateVirtualIpReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	v, err := Svc.Create(ctx, vipsvc.CreateCommand{OrganizationID: req.GetOrganizationId(), Scope: req.GetScope(), Name: req.GetName(),
		Description: req.GetDescription(), AvatarObjectPath: req.GetAvatarObjectPath(), FullBodyObjectPath: req.GetFullBodyObjectPath(),
		ThreeViewObjectPath: req.GetThreeViewObjectPath(), NineViewObjectPath: req.GetNineViewObjectPath(),
		PersonaProfile: req.GetPersonaProfile(), OwnerUserID: auth.CallerID(ctx), Visibility: req.GetVisibility()})
	if err != nil {
		httperr.Write(c, err, 400, "virtual_ip")
		return
	}
	c.JSON(201, toView(v))
}
func GetVirtualIp(ctx context.Context, c *app.RequestContext) {
	var req pb.GetVirtualIpReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	v, err := Svc.Get(ctx, req.GetId(), auth.CallerID(ctx), auth.TeamsFromContext(ctx))
	if err != nil || v.OrganizationID != req.GetOrganizationId() {
		c.String(404, "virtual_ip not found")
		return
	}
	c.JSON(200, toView(v))
}
func ListVirtualIps(ctx context.Context, c *app.RequestContext) {
	var req pb.ListVirtualIpsReq
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
	items, total, err := Svc.List(ctx, vipsvc.ListQuery{OrganizationID: req.GetOrganizationId(), Scope: req.GetScope(), Name: req.GetName(),
		Status: req.GetStatus(), Visibility: req.GetVisibility(), Limit: size, Offset: (page - 1) * size}, auth.CallerID(ctx), auth.TeamsFromContext(ctx))
	if err != nil {
		httperr.Write(c, err, 500, "virtual_ip")
		return
	}
	rows := make([]*pb.VirtualIp, len(items))
	for i := range items {
		rows[i] = toView(items[i])
	}
	c.JSON(200, &pb.ListVirtualIpsResp{Items: rows, Total: int32(total)})
}
func UpdateVirtualIp(ctx context.Context, c *app.RequestContext) {
	var req pb.UpdateVirtualIpReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	v, err := Svc.Update(ctx, vipsvc.UpdateCommand{ID: req.GetId(), OrganizationID: req.GetOrganizationId(), Name: req.GetName(),
		Description: req.GetDescription(), AvatarObjectPath: req.GetAvatarObjectPath(), FullBodyObjectPath: req.GetFullBodyObjectPath(),
		ThreeViewObjectPath: req.GetThreeViewObjectPath(), NineViewObjectPath: req.GetNineViewObjectPath(),
		PersonaProfile: req.GetPersonaProfile(), CallerID: auth.CallerID(ctx)})
	if err != nil {
		httperr.Write(c, err, 400, "virtual_ip")
		return
	}
	c.JSON(200, toView(v))
}
func ArchiveVirtualIp(ctx context.Context, c *app.RequestContext) {
	var req pb.GetVirtualIpReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	v, err := Svc.Archive(ctx, req.GetId(), req.GetOrganizationId(), auth.CallerID(ctx))
	if err != nil {
		httperr.Write(c, err, 404, "virtual_ip")
		return
	}
	c.JSON(200, toView(v))
}
func DeleteVirtualIp(ctx context.Context, c *app.RequestContext) {
	var req pb.GetVirtualIpReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	v, err := Svc.Delete(ctx, req.GetId(), req.GetOrganizationId(), auth.CallerID(ctx))
	if err != nil {
		httperr.Write(c, err, 400, "virtual_ip")
		return
	}
	c.JSON(200, toView(v))
}
func toView(v *domain.VirtualIp) *pb.VirtualIp {
	return &pb.VirtualIp{Id: v.ID, OrganizationId: v.OrganizationID, Scope: v.Scope, Name: v.Name, Description: v.Description,
		AvatarObjectPath: v.AvatarObjectPath, FullBodyObjectPath: v.FullBodyObjectPath, ThreeViewObjectPath: v.ThreeViewObjectPath,
		NineViewObjectPath: v.NineViewObjectPath, PersonaProfile: v.PersonaProfile, Status: string(v.Status),
		Visibility: string(v.Visibility), OwnerUserId: v.OwnerUserID, CreatedAt: v.CreatedAt.UnixNano(), UpdatedAt: v.UpdatedAt.UnixNano()}
}

func requireOrganization(ctx context.Context, c *app.RequestContext, id string) bool {
	if auth.HasOrganization(ctx, id) {
		return true
	}
	c.String(404, "organization not found")
	return false
}
