package organization

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	pb "media_agent/hertz_gen/model/media_auth/organization"
	"media_agent/media_auth/biz/middleware"
	service "media_agent/media_auth/biz/service/organization"
)

var Svc *service.Service

func caller(ctx context.Context, c *app.RequestContext) (string, string, bool) {
	s := middleware.SessionFromContext(ctx)
	if s == nil {
		c.JSON(401, map[string]string{"error": "missing session"})
		return "", "", false
	}
	return s.Sub, s.Role, true
}

func ListOrganizations(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	rows, err := Svc.List(ctx, uid)
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]*pb.ApiOrganization, 0, len(rows))
	for _, v := range rows {
		out = append(out, toDTO(v))
	}
	c.JSON(200, &pb.ListOrganizationsResponse{Organizations: out})
}
func CreateOrganization(ctx context.Context, c *app.RequestContext) {
	uid, role, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.CreateOrganizationRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	v, err := Svc.Create(ctx, uid, role, req.GetName())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(201, toDTO(v))
}
func GetOrganization(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.GetOrganizationRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	v, err := Svc.Get(ctx, uid, req.GetId())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, toDTO(v))
}
func UpdateOrganization(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.UpdateOrganizationRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	v, err := Svc.Update(ctx, uid, req.GetId(), req.Name, req.Status)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, toDTO(v))
}
func ListMembers(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.ListMembersRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	rows, err := Svc.ListMembers(ctx, uid, req.GetId())
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]*pb.ApiOrganizationMember, 0, len(rows))
	for _, v := range rows {
		out = append(out, memberDTO(v))
	}
	c.JSON(200, &pb.ListMembersResponse{Members: out})
}
func AddMember(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.AddMemberRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	role := "member"
	if req.Role != nil {
		role = *req.Role
	}
	v, err := Svc.PutMember(ctx, uid, req.GetId(), req.GetUserId(), role)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, memberDTO(v))
}
func UpdateMember(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.UpdateMemberRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	v, err := Svc.PutMember(ctx, uid, req.GetId(), req.GetUserId(), req.GetRole())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, memberDTO(v))
}
func RemoveMember(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.RemoveMemberRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	if err := Svc.RemoveMember(ctx, uid, req.GetId(), req.GetUserId()); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, &pb.MutationResponse{Success: true})
}
func GetEntitlements(ctx context.Context, c *app.RequestContext) {
	uid, _, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.GetEntitlementsRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	keys, err := Svc.GetEntitlements(ctx, uid, req.GetId())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, &pb.EntitlementsResponse{FeatureKeys: keys})
}
func SetEntitlements(ctx context.Context, c *app.RequestContext) {
	uid, role, ok := caller(ctx, c)
	if !ok {
		return
	}
	var req pb.SetEntitlementsRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.String(400, err.Error())
		return
	}
	keys, err := Svc.SetEntitlements(ctx, uid, role, req.GetId(), req.GetFeatureKeys())
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(200, &pb.EntitlementsResponse{FeatureKeys: keys})
}

func toDTO(v service.Summary) *pb.ApiOrganization {
	return &pb.ApiOrganization{Id: v.ID, Name: v.Name, Status: v.Status, Version: v.Version, MyRole: v.MyRole, CreatedAt: v.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), UpdatedAt: v.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")}
}
func memberDTO(v service.Membership) *pb.ApiOrganizationMember {
	return &pb.ApiOrganizationMember{UserId: v.UserID, UserName: v.UserName, Role: v.Role, JoinedAt: v.JoinedAt.Format("2006-01-02T15:04:05Z07:00")}
}
func writeErr(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrNotMember):
		c.JSON(consts.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrForbidden), errors.Is(err, service.ErrPlatformAdmin):
		c.JSON(consts.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrLastAdmin):
		c.JSON(consts.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidName), errors.Is(err, service.ErrInvalidRole), errors.Is(err, service.ErrInvalidStatus), errors.Is(err, service.ErrUserNotFound):
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}
