// Handlers are maintained here; routes and API models are generated from IDL.
package prompt

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_control/prompt"
	domain "media_agent/media_control/biz/domain/prompt"
	"media_agent/media_control/biz/middleware"
	service "media_agent/media_control/biz/service/prompt"
)

var promptService *service.Service

func Configure(value *service.Service) { promptService = value }
func bind(c *app.RequestContext, value any) bool {
	if promptService == nil {
		c.JSON(503, map[string]string{"error": "prompt_unavailable"})
		return false
	}
	if err := c.BindAndValidate(value); err != nil {
		c.JSON(400, map[string]string{"error": "invalid_request"})
		return false
	}
	return true
}
func grants(values []*pb.PromptGrant) ([]domain.Grant, error) {
	out := make([]domain.Grant, 0, len(values))
	for _, v := range values {
		if v == nil {
			return nil, domain.ErrInvalid
		}
		out = append(out, domain.Grant{TeamID: v.TeamId, Permission: v.Permission})
	}
	return out, nil
}
func CreatePrompt(ctx context.Context, c *app.RequestContext) {
	var req pb.CreatePromptRequest
	if !bind(c, &req) {
		return
	}
	if req.WorkspaceId != "" {
		writeError(c, domain.ErrInvalid)
		return
	}
	acl, err := grants(req.Grants)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := promptService.Create(ctx, service.CreateCommand{OwnerUserID: middleware.UserID(ctx), OrganizationID: req.OrganizationId, Name: req.Name, Content: req.Content, Visibility: req.Visibility, Grants: acl})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(201, toResponse(v))
}
func ListPrompts(ctx context.Context, c *app.RequestContext) {
	var req pb.ListPromptsRequest
	if !bind(c, &req) {
		return
	}
	if req.OrganizationId == "" || req.WorkspaceId != "" {
		writeError(c, domain.ErrInvalid)
		return
	}
	values, err := promptService.List(ctx, middleware.UserID(ctx), req.OrganizationId)
	if err != nil {
		writeError(c, err)
		return
	}
	resp := &pb.ListPromptsResponse{Prompts: make([]*pb.PromptResponse, 0, len(values))}
	for i := range values {
		resp.Prompts = append(resp.Prompts, toResponse(&values[i]))
	}
	c.JSON(200, resp)
}
func GetPrompt(ctx context.Context, c *app.RequestContext) {
	var req pb.GetPromptRequest
	if !bind(c, &req) {
		return
	}
	v, err := promptService.Get(ctx, req.PromptId, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, toResponse(v))
}
func UpdatePrompt(ctx context.Context, c *app.RequestContext) {
	var req pb.UpdatePromptRequest
	if !bind(c, &req) {
		return
	}
	v, err := promptService.Update(ctx, service.UpdateCommand{PromptID: req.PromptId, OwnerUserID: middleware.UserID(ctx), Name: req.Name, Content: req.Content, ExpectedVersion: req.ExpectedVersion})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, toResponse(v))
}
func ArchivePrompt(ctx context.Context, c *app.RequestContext) {
	var req pb.ArchivePromptRequest
	if !bind(c, &req) {
		return
	}
	v, err := promptService.Archive(ctx, service.ArchiveCommand{PromptID: req.PromptId, OwnerUserID: middleware.UserID(ctx), ExpectedVersion: req.ExpectedVersion})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, toResponse(v))
}
func SetPromptAccess(ctx context.Context, c *app.RequestContext) {
	var req pb.SetPromptAccessRequest
	if !bind(c, &req) {
		return
	}
	acl, err := grants(req.Grants)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := promptService.SetACL(ctx, service.ACLCommand{PromptID: req.PromptId, UserID: middleware.UserID(ctx), Visibility: req.Visibility, ExpectedVersion: req.ExpectedVersion, Grants: acl})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, toResponse(v))
}
func ListPromptVersions(ctx context.Context, c *app.RequestContext) {
	var req pb.GetPromptRequest
	if !bind(c, &req) {
		return
	}
	values, err := promptService.Versions(ctx, req.PromptId, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	resp := &pb.ListPromptVersionsResponse{Versions: make([]*pb.PromptVersion, 0, len(values))}
	for _, v := range values {
		resp.Versions = append(resp.Versions, toVersion(v))
	}
	c.JSON(200, resp)
}
func GetPromptVersion(ctx context.Context, c *app.RequestContext) {
	var req pb.GetPromptVersionRequest
	if !bind(c, &req) {
		return
	}
	v, err := promptService.Version(ctx, req.PromptId, middleware.UserID(ctx), req.Version)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, toVersion(*v))
}
func RollbackPrompt(ctx context.Context, c *app.RequestContext) {
	var req pb.RollbackPromptRequest
	if !bind(c, &req) {
		return
	}
	v, err := promptService.Rollback(ctx, service.RollbackCommand{PromptID: req.PromptId, UserID: middleware.UserID(ctx), ExpectedVersion: req.ExpectedVersion, TargetVersion: req.TargetVersion})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, toResponse(v))
}
func toResponse(v *service.View) *pb.PromptResponse {
	result := &pb.PromptResponse{Id: v.ID, OrganizationId: v.OrganizationID, OwnerUserId: v.OwnerUserID, Name: v.Name, Content: v.Content, Status: string(v.Status), Version: v.Version, CreatedAt: v.CreatedAt.UnixMilli(), UpdatedAt: v.UpdatedAt.UnixMilli(), Visibility: v.Visibility, MyPermission: v.MyPermission, Grants: make([]*pb.PromptGrant, 0, len(v.Grants))}
	for _, g := range v.Grants {
		result.Grants = append(result.Grants, &pb.PromptGrant{TeamId: g.TeamID, Permission: g.Permission})
	}
	return result
}
func toVersion(v domain.Version) *pb.PromptVersion {
	result := &pb.PromptVersion{Version: v.Version, Name: v.Name, Content: v.Content}
	if !v.CreatedAt.IsZero() {
		result.CreatedAt = v.CreatedAt.UnixMilli()
	}
	return result
}
func writeError(c *app.RequestContext, err error) {
	status := 500
	message := "prompt_request_failed"
	switch {
	case errors.Is(err, domain.ErrInvalid):
		status = 400
		message = err.Error()
	case errors.Is(err, domain.ErrNotFound):
		status = 404
		message = domain.ErrNotFound.Error()
	case errors.Is(err, domain.ErrForbidden):
		status = 403
		message = domain.ErrForbidden.Error()
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrArchived):
		status = 409
		message = err.Error()
	case errors.Is(err, domain.ErrUnavailable):
		status = 503
		message = domain.ErrUnavailable.Error()
	}
	c.JSON(status, map[string]string{"error": message})
}

// ListUnassignedPrompts .
// @router /api/v1/prompts/unassigned [GET]
func ListUnassignedPrompts(ctx context.Context, c *app.RequestContext) {
	var req pb.ListUnassignedPromptsRequest
	if !bind(c, &req) {
		return
	}
	values, err := promptService.Unassigned(ctx, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	resp := &pb.ListPromptsResponse{Prompts: make([]*pb.PromptResponse, 0, len(values))}
	for i := range values {
		resp.Prompts = append(resp.Prompts, toResponse(&values[i]))
	}
	c.JSON(200, resp)
}

// AssignPromptOrganization .
// @router /api/v1/prompts/:prompt_id/assign-organization [POST]
func AssignPromptOrganization(ctx context.Context, c *app.RequestContext) {
	var req pb.AssignPromptOrganizationRequest
	if !bind(c, &req) {
		return
	}
	v, err := promptService.AssignOrganization(ctx, req.PromptId, middleware.UserID(ctx), req.OrganizationId, req.ExpectedVersion)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, toResponse(v))
}
