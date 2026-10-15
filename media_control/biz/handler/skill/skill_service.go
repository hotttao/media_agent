// Handlers are maintained here; API models and routes are generated from IDL.
package skill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_control/skill"
	d "media_agent/media_control/biz/domain/skill"
	"media_agent/media_control/biz/middleware"
	service "media_agent/media_control/biz/service/skill"
	flow "media_agent/media_control/biz/workflow/skill_content"
	"sort"
	"unicode/utf8"
)

var application *flow.Flow

func Configure(value *flow.Flow) { application = value }
func bind(c *app.RequestContext, value any) bool {
	if application == nil {
		c.JSON(503, map[string]string{"error": "skill_unavailable"})
		return false
	}
	if err := c.BindAndValidate(value); err != nil {
		c.JSON(400, map[string]string{"error": "invalid_request"})
		return false
	}
	return true
}
func grants(values []*pb.SkillGrant) ([]d.Grant, error) {
	result := make([]d.Grant, 0, len(values))
	for _, v := range values {
		if v == nil {
			return nil, d.ErrInvalid
		}
		result = append(result, d.Grant{TeamID: v.TeamId, Permission: v.Permission})
	}
	return result, nil
}
func gitInput(value *pb.GitSourceInput) *flow.GitInput {
	if value == nil {
		return nil
	}
	return &flow.GitInput{URL: value.Url, Branch: value.Branch, Subpath: value.Subpath, AccessToken: value.AccessToken}
}
func result(c *app.RequestContext, v *service.View, err error, status int) {
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(status, toResponse(v))
}
func create(ctx context.Context, c *app.RequestContext, requireGit bool) {
	var req pb.CreateSkillRequest
	if !bind(c, &req) {
		return
	}
	if req.WorkspaceId != "" || req.ContentPath != "" || req.Checksum != "" || requireGit && req.Git == nil {
		writeError(c, d.ErrInvalid)
		return
	}
	acl, err := grants(req.Grants)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := application.Create(ctx, flow.Create{UserID: middleware.UserID(ctx), OrganizationID: req.OrganizationId, Name: req.Name, Description: req.Description, Visibility: req.Visibility, Grants: acl, Slug: req.Slug, Archive: req.Content, Git: gitInput(req.Git)})
	result(c, v, err, 201)
}
func CreateSkill(ctx context.Context, c *app.RequestContext)    { create(ctx, c, false) }
func ImportGitSkill(ctx context.Context, c *app.RequestContext) { create(ctx, c, true) }
func ListSkills(ctx context.Context, c *app.RequestContext) {
	var req pb.ListSkillsRequest
	if !bind(c, &req) {
		return
	}
	if req.OrganizationId == "" || req.WorkspaceId != "" {
		writeError(c, d.ErrInvalid)
		return
	}
	values, err := application.Skills.List(ctx, middleware.UserID(ctx), req.OrganizationId)
	if err != nil {
		writeError(c, err)
		return
	}
	listResponse(c, values)
}
func ListUnassignedSkills(ctx context.Context, c *app.RequestContext) {
	var req pb.EmptyRequest
	if !bind(c, &req) {
		return
	}
	values, err := application.Skills.Unassigned(ctx, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	listResponse(c, values)
}
func listResponse(c *app.RequestContext, values []service.View) {
	response := &pb.ListSkillsResponse{Skills: make([]*pb.SkillResponse, 0, len(values))}
	for i := range values {
		response.Skills = append(response.Skills, toResponse(&values[i]))
	}
	c.JSON(200, response)
}
func GetSkill(ctx context.Context, c *app.RequestContext) {
	var req pb.GetSkillRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.Skills.Get(ctx, req.SkillId, middleware.UserID(ctx))
	result(c, v, err, 200)
}
func UpdateSkill(ctx context.Context, c *app.RequestContext) {
	var req pb.UpdateSkillRequest
	if !bind(c, &req) {
		return
	}
	if req.ContentPath != "" || req.Checksum != "" {
		writeError(c, d.ErrInvalid)
		return
	}
	v, err := application.Skills.Update(ctx, service.UpdateCommand{SkillID: req.SkillId, OwnerUserID: middleware.UserID(ctx), Name: req.Name, Description: req.Description, ExpectedVersion: req.ExpectedVersion})
	result(c, v, err, 200)
}
func SetSkillStatus(ctx context.Context, c *app.RequestContext) {
	var req pb.SetSkillStatusRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.Skills.SetStatus(ctx, service.StatusCommand{SkillID: req.SkillId, OwnerUserID: middleware.UserID(ctx), Status: req.Status, ExpectedVersion: req.ExpectedVersion})
	result(c, v, err, 200)
}
func ArchiveSkill(ctx context.Context, c *app.RequestContext) {
	var req pb.ArchiveSkillRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.Skills.Archive(ctx, service.ArchiveCommand{SkillID: req.SkillId, OwnerUserID: middleware.UserID(ctx), ExpectedVersion: req.ExpectedVersion})
	result(c, v, err, 200)
}
func SetSkillAccess(ctx context.Context, c *app.RequestContext) {
	var req pb.SetSkillAccessRequest
	if !bind(c, &req) {
		return
	}
	acl, err := grants(req.Grants)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := application.Skills.SetACL(ctx, service.ACLCommand{SkillID: req.SkillId, UserID: middleware.UserID(ctx), Visibility: req.Visibility, Grants: acl, ExpectedVersion: req.ExpectedVersion})
	result(c, v, err, 200)
}
func AssignSkillOrganization(ctx context.Context, c *app.RequestContext) {
	var req pb.AssignSkillOrganizationRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.Skills.AssignOrganization(ctx, req.SkillId, middleware.UserID(ctx), req.OrganizationId, req.ExpectedVersion)
	result(c, v, err, 200)
}
func binary(content []byte) bool { return !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 }
func ListSkillFiles(ctx context.Context, c *app.RequestContext) {
	var req pb.SkillFileQuery
	if !bind(c, &req) {
		return
	}
	v, tree, err := application.Tree(ctx, req.SkillId, middleware.UserID(ctx), req.Version)
	if err != nil {
		writeError(c, err)
		return
	}
	names := make([]string, 0, len(tree))
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)
	response := &pb.SkillFilesResponse{ExpectedVersion: v.Version, Files: make([]*pb.SkillFile, 0, len(names))}
	for _, name := range names {
		file := tree[name]
		response.Files = append(response.Files, &pb.SkillFile{Path: name, Size: int64(len(file.Content)), Binary: binary(file.Content), Executable: file.Executable})
	}
	c.JSON(200, response)
}
func GetSkillFile(ctx context.Context, c *app.RequestContext) {
	var req pb.SkillFileQuery
	if !bind(c, &req) {
		return
	}
	v, tree, err := application.Tree(ctx, req.SkillId, middleware.UserID(ctx), req.Version)
	if err != nil {
		writeError(c, err)
		return
	}
	file, ok := tree[req.Path]
	if !ok {
		writeError(c, d.ErrNotFound)
		return
	}
	c.JSON(200, &pb.SkillFileResponse{Path: req.Path, Content: file.Content, Binary: binary(file.Content), Executable: file.Executable, ExpectedVersion: v.Version})
}
func PutSkillFile(ctx context.Context, c *app.RequestContext) {
	var req pb.PutSkillFileRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.WriteFile(ctx, req.SkillId, middleware.UserID(ctx), req.Path, req.ExpectedVersion, req.Content, req.Executable)
	result(c, v, err, 200)
}
func DeleteSkillFile(ctx context.Context, c *app.RequestContext) {
	var req pb.DeleteSkillFileRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.DeleteFile(ctx, req.SkillId, middleware.UserID(ctx), req.Path, req.ExpectedVersion)
	result(c, v, err, 200)
}
func UploadSkillDraft(ctx context.Context, c *app.RequestContext) {
	var req pb.UploadSkillDraftRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.UploadDraft(ctx, req.SkillId, middleware.UserID(ctx), req.ExpectedVersion, req.Content)
	result(c, v, err, 200)
}
func DiscardSkillDraft(ctx context.Context, c *app.RequestContext) {
	var req pb.SkillWriteRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.DiscardDraft(ctx, req.SkillId, middleware.UserID(ctx), req.ExpectedVersion)
	result(c, v, err, 200)
}
func PublishSkill(ctx context.Context, c *app.RequestContext) {
	var req pb.SkillWriteRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.Publish(ctx, req.SkillId, middleware.UserID(ctx), req.ExpectedVersion)
	result(c, v, err, 200)
}
func ListSkillVersions(ctx context.Context, c *app.RequestContext) {
	var req pb.GetSkillRequest
	if !bind(c, &req) {
		return
	}
	values, err := application.Skills.Versions(ctx, req.SkillId, middleware.UserID(ctx))
	if err != nil {
		writeError(c, err)
		return
	}
	response := &pb.SkillVersionsResponse{Versions: make([]*pb.SkillVersion, 0, len(values))}
	for _, v := range values {
		response.Versions = append(response.Versions, &pb.SkillVersion{Version: v.Version, Checksum: v.Checksum, Slug: v.Slug, Description: v.Description, PublishedBy: v.PublishedBy, CreatedAt: v.CreatedAt.UnixMilli()})
	}
	c.JSON(200, response)
}
func GetSkillPackage(ctx context.Context, c *app.RequestContext) {
	var req pb.SkillVersionQuery
	if !bind(c, &req) {
		return
	}
	v, body, err := application.Package(ctx, req.SkillId, middleware.UserID(ctx), req.Version)
	if err != nil {
		writeError(c, err)
		return
	}
	slug := v.Slug
	if !d.IsSlug(slug) {
		slug = "skill"
	}
	c.JSON(200, &pb.SkillPackageResponse{Content: body, Checksum: v.Checksum, Filename: fmt.Sprintf("%s-v%d.zip", slug, v.Version)})
}
func SetSkillSource(ctx context.Context, c *app.RequestContext) {
	var req pb.SetSkillSourceRequest
	if !bind(c, &req) {
		return
	}
	input := gitInput(req.Source)
	if input == nil {
		writeError(c, d.ErrInvalid)
		return
	}
	v, err := application.UpdateSource(ctx, req.SkillId, middleware.UserID(ctx), req.ExpectedVersion, *input)
	result(c, v, err, 200)
}
func SyncGitSkill(ctx context.Context, c *app.RequestContext) {
	var req pb.SkillWriteRequest
	if !bind(c, &req) {
		return
	}
	v, err := application.Sync(ctx, req.SkillId, middleware.UserID(ctx), req.ExpectedVersion)
	result(c, v, err, 200)
}
func toResponse(v *service.View) *pb.SkillResponse {
	response := &pb.SkillResponse{Id: v.ID, OrganizationId: v.OrganizationID, OwnerUserId: v.OwnerUserID, Name: v.Name, Description: v.Description, Visibility: v.Visibility, MyPermission: v.MyPermission, Grants: make([]*pb.SkillGrant, 0, len(v.Grants)), ContentPath: v.ContentPath, Checksum: v.Checksum, DraftChecksum: v.DraftChecksum, PublishedVersion: v.PublishedVersion, Version: v.Version, Status: string(v.Status), SourceKind: v.SourceKind, HasDraft: v.DraftChecksum != "" && v.DraftChecksum != v.Checksum, Source: &pb.GitSource{Url: v.Source.URL, Branch: v.Source.Branch, Subpath: v.Source.Subpath, Commit: v.Source.Commit, CredentialConfigured: len(v.Source.EncryptedToken) > 0}, CreatedAt: v.CreatedAt.UnixMilli(), UpdatedAt: v.UpdatedAt.UnixMilli()}
	for _, g := range v.Grants {
		response.Grants = append(response.Grants, &pb.SkillGrant{TeamId: g.TeamID, Permission: g.Permission})
	}
	return response
}
func writeError(c *app.RequestContext, err error) {
	status := 503
	message := "skill_content_unavailable"
	switch {
	case errors.Is(err, d.ErrNotFound):
		status = 404
		message = d.ErrNotFound.Error()
	case errors.Is(err, d.ErrForbidden):
		status = 403
		message = d.ErrForbidden.Error()
	case errors.Is(err, d.ErrInvalid), errors.Is(err, d.ErrInvalidStatus):
		status = 400
		message = d.ErrInvalid.Error()
	case errors.Is(err, d.ErrConflict), errors.Is(err, d.ErrArchived):
		status = 409
		message = err.Error()
	}
	c.JSON(status, map[string]string{"error": message})
}
