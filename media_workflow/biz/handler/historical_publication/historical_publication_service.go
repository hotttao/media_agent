package historical_publication

import (
	"context"
	"errors"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"gorm.io/gorm"

	pb "media_agent/hertz_gen/model/media_workflow/historical_publication"
	publication "media_agent/media_workflow/biz/domain/historical_publication"
	service "media_agent/media_workflow/biz/service/historical_publication"
	"media_agent/media_workflow/biz/shared/auth"
)

var Svc *service.Service

// ImportHistoricalVideoPush imports a frozen source fact, not a publishing command.
// @router /api/internal/v1/historical-video-pushes/import [POST]
func ImportHistoricalVideoPush(ctx context.Context, c *app.RequestContext) {
	var req pb.ImportHistoricalVideoPushReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(consts.StatusBadRequest, err.Error())
		return
	}
	created, err := time.Parse(time.RFC3339Nano, req.GetSourceCreatedAt())
	if err != nil {
		c.String(consts.StatusBadRequest, "source_created_at must be RFC3339")
		return
	}
	updated, err := time.Parse(time.RFC3339Nano, req.GetSourceUpdatedAt())
	if err != nil {
		c.String(consts.StatusBadRequest, "source_updated_at must be RFC3339")
		return
	}
	members := make([]publication.Member, 0, len(req.GetMembers()))
	for _, member := range req.GetMembers() {
		members = append(members, publication.Member{Ordinal: int(member.GetOrdinal()), SourceVideoID: member.GetSourceVideoId(), JobID: member.GetJobId()})
	}
	item, err := Svc.Import(ctx, service.ImportCommand{
		OrganizationID: req.GetOrganizationId(), WorkspaceID: req.GetWorkspaceId(), OwnerUserID: req.GetOwnerUserId(),
		SourceRecordID: req.GetSourceRecordId(), ProductID: req.GetProductId(), VirtualIPID: req.GetVirtualIpId(),
		SourceSceneID: req.GetSourceSceneId(), TemplateName: req.GetTemplateName(), Title: req.GetTitle(), Content: req.GetContent(),
		SourceStatus: req.GetSourceStatus(), IsQualified: req.GetIsQualified(), IsPublished: req.GetIsPublished(),
		Video: fromMediaPB(req.GetVideo()), Thumbnail: fromMediaPB(req.GetThumbnail()), ManualClip: fromMediaPB(req.GetManualClip()),
		SourceCreatedAt: created, SourceUpdatedAt: updated, Members: members,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConflict):
			c.String(consts.StatusConflict, err.Error())
		case errors.Is(err, service.ErrForbidden):
			c.String(consts.StatusForbidden, err.Error())
		default:
			c.String(consts.StatusBadRequest, err.Error())
		}
		return
	}
	c.JSON(consts.StatusOK, &pb.HistoricalVideoPushResp{Item: toPB(item)})
}

// GetHistoricalVideoPush returns a Workspace-authorized record with fresh media URLs.
// @router /api/v1/historical-video-pushes/:id [GET]
func GetHistoricalVideoPush(ctx context.Context, c *app.RequestContext) {
	var req pb.GetHistoricalVideoPushReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(consts.StatusBadRequest, err.Error())
		return
	}
	item, err := Svc.Get(ctx, req.GetId(), auth.CallerID(ctx))
	if err != nil {
		if errors.Is(err, service.ErrForbidden) || errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(consts.StatusNotFound, "historical VideoPush not found")
		} else {
			c.String(consts.StatusServiceUnavailable, "historical VideoPush unavailable")
		}
		return
	}
	c.JSON(consts.StatusOK, &pb.HistoricalVideoPushResp{Item: toPB(item)})
}

// ListHistoricalVideoPushes lists records of an accessible Workspace.
// @router /api/v1/historical-video-pushes [GET]
func ListHistoricalVideoPushes(ctx context.Context, c *app.RequestContext) {
	var req pb.ListHistoricalVideoPushesReq
	if err := c.BindAndValidate(&req); err != nil {
		c.String(consts.StatusBadRequest, err.Error())
		return
	}
	page, pageSize := int(req.GetPage()), int(req.GetPageSize())
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}
	items, total, err := Svc.List(ctx, req.GetWorkspaceId(), auth.CallerID(ctx), page, pageSize)
	if err != nil {
		if errors.Is(err, service.ErrForbidden) {
			c.String(consts.StatusNotFound, "Workspace not found")
		} else {
			c.String(consts.StatusBadRequest, err.Error())
		}
		return
	}
	out := &pb.ListHistoricalVideoPushesResp{Items: make([]*pb.HistoricalVideoPush, 0, len(items)), Total: total}
	for _, item := range items {
		out.Items = append(out.Items, toPB(item))
	}
	c.JSON(consts.StatusOK, out)
}

func fromMediaPB(value *pb.HistoricalMediaObject) publication.MediaObject {
	if value == nil {
		return publication.MediaObject{}
	}
	return publication.MediaObject{ObjectPath: value.GetObjectPath(), SHA256: value.GetSha256()}
}

func toMediaPB(value publication.MediaObject) *pb.HistoricalMediaObject {
	return &pb.HistoricalMediaObject{ObjectPath: value.ObjectPath, Sha256: value.SHA256, AccessUrl: value.AccessURL}
}

func toPB(value *publication.HistoricalVideoPush) *pb.HistoricalVideoPush {
	if value == nil {
		return nil
	}
	item := &pb.HistoricalVideoPush{
		Id: value.ID, OrganizationId: value.OrganizationID, WorkspaceId: value.WorkspaceID, OwnerUserId: value.OwnerUserID,
		SourceRecordId: value.SourceRecordID, ProductId: value.ProductID, VirtualIpId: value.VirtualIPID,
		SourceSceneId: value.SourceSceneID, TemplateName: value.TemplateName, Title: value.Title, Content: value.Content,
		SourceStatus: value.SourceStatus, IsQualified: value.IsQualified, IsPublished: value.IsPublished,
		Video: toMediaPB(value.Video), Thumbnail: toMediaPB(value.Thumbnail), ManualClip: toMediaPB(value.ManualClip),
		SourceCreatedAt: value.SourceCreatedAt.UTC().Format(time.RFC3339Nano), SourceUpdatedAt: value.SourceUpdatedAt.UTC().Format(time.RFC3339Nano),
		ImportedAt: value.ImportedAt.UTC().Format(time.RFC3339Nano),
		Members:    make([]*pb.HistoricalVideoPushMember, 0, len(value.Members)),
	}
	for _, member := range value.Members {
		item.Members = append(item.Members, &pb.HistoricalVideoPushMember{Ordinal: int32(member.Ordinal), SourceVideoId: member.SourceVideoID, JobId: member.JobID})
	}
	return item
}
