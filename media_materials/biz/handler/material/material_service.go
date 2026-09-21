package material

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	pb "media_agent/hertz_gen/model/media_materials/material"
	domain "media_agent/media_materials/biz/domain/material"
	materialsvc "media_agent/media_materials/biz/service/material"
	"media_agent/media_materials/biz/shared/auth"
	"media_agent/media_materials/biz/shared/httperr"
)

var Svc *materialsvc.Service

func CreateMaterial(ctx context.Context, c *app.RequestContext) {
	var req pb.CreateMaterialReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	m, err := Svc.Create(ctx, materialsvc.CreateCommand{
		OrganizationID: req.GetOrganizationId(), Scope: req.GetScope(), Name: req.GetName(),
		Description: req.GetDescription(), Prompt: req.GetPrompt(), AssetKind: req.GetAssetKind(),
		ObjectPath: req.GetObjectPath(), Content: req.GetContent(), Metadata: req.GetMetadata(),
		OwnerUserID: auth.CallerID(ctx), Visibility: req.GetVisibility(), TeamID: req.GetTeamId(),
	})
	if err != nil {
		httperr.Write(c, err, 400, "material")
		return
	}
	c.JSON(consts.StatusCreated, toView(m))
}

func GetMaterial(ctx context.Context, c *app.RequestContext) {
	var req pb.GetMaterialReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	m, err := Svc.Get(ctx, req.GetId(), auth.CallerID(ctx), auth.TeamsFromContext(ctx))
	if err != nil || m.OrganizationID != req.GetOrganizationId() {
		c.String(404, "material not found")
		return
	}
	c.JSON(200, toView(m))
}

func ListMaterials(ctx context.Context, c *app.RequestContext) {
	var req pb.ListMaterialsReq
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
	onlyMine := req.OnlyMine == nil || req.GetOnlyMine()
	owner := ""
	if onlyMine {
		owner = auth.CallerID(ctx)
	}
	items, total, err := Svc.List(ctx, materialsvc.ListQuery{OrganizationID: req.GetOrganizationId(), Scope: req.GetScope(),
		AssetKind: req.GetAssetKind(), Owner: owner, Visibility: req.GetVisibility(), Limit: size, Offset: (page - 1) * size},
		auth.CallerID(ctx), auth.TeamsFromContext(ctx))
	if err != nil {
		httperr.Write(c, err, 500, "material")
		return
	}
	rows := make([]*pb.Material, len(items))
	for i := range items {
		rows[i] = toView(items[i])
	}
	c.JSON(200, &pb.ListMaterialsResp{Items: rows, Total: int32(total)})
}

func UpdateMaterial(ctx context.Context, c *app.RequestContext) {
	var req pb.UpdateMaterialReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	m, err := Svc.Update(ctx, materialsvc.UpdateCommand{ID: req.GetId(), OrganizationID: req.GetOrganizationId(),
		Name: req.GetName(), Description: req.GetDescription(), Prompt: req.GetPrompt(), ObjectPath: req.GetObjectPath(),
		Content: req.GetContent(), Metadata: req.GetMetadata(), Visibility: req.GetVisibility(), TeamID: req.GetTeamId(), CallerID: auth.CallerID(ctx)})
	if err != nil {
		httperr.Write(c, err, 400, "material")
		return
	}
	c.JSON(200, toView(m))
}

func DeleteMaterial(ctx context.Context, c *app.RequestContext) {
	var req pb.GetMaterialReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if !requireOrganization(ctx, c, req.GetOrganizationId()) {
		return
	}
	m, err := Svc.Delete(ctx, req.GetId(), req.GetOrganizationId(), auth.CallerID(ctx))
	if err != nil {
		httperr.Write(c, err, 400, "material")
		return
	}
	c.JSON(200, toView(m))
}

func toView(m *domain.Material) *pb.Material {
	return &pb.Material{Id: m.ID, OrganizationId: m.OrganizationID, Scope: m.Scope, Name: m.Name,
		Description: m.Description, Prompt: m.Prompt, AssetKind: m.AssetKind, ObjectPath: m.ObjectPath,
		Content: m.Content, Metadata: m.Metadata, OwnerUserId: m.OwnerUserID, Visibility: string(m.Visibility),
		TeamId: m.TeamID, CreatedAt: m.CreatedAt.UnixNano(), UpdatedAt: m.UpdatedAt.UnixNano()}
}

func requireOrganization(ctx context.Context, c *app.RequestContext, id string) bool {
	if auth.HasOrganization(ctx, id) {
		return true
	}
	c.String(consts.StatusNotFound, "organization not found")
	return false
}
