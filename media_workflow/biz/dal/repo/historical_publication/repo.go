package historical_publication

import (
	"context"
	"time"

	"gorm.io/gorm"

	publication "media_agent/media_workflow/biz/domain/historical_publication"
)

type Repo struct{ db *gorm.DB }

func New(db *gorm.DB) *Repo { return &Repo{db: db} }

type pushRow struct {
	ID                   string    `gorm:"column:id;primaryKey"`
	OrganizationID       string    `gorm:"column:organization_id"`
	WorkspaceID          string    `gorm:"column:workspace_id"`
	OwnerUserID          string    `gorm:"column:owner_user_id"`
	SourceSystem         string    `gorm:"column:source_system"`
	SourceRecordID       string    `gorm:"column:source_record_id"`
	SourceDigest         string    `gorm:"column:source_digest"`
	ProductID            string    `gorm:"column:product_id"`
	VirtualIPID          string    `gorm:"column:virtual_ip_id"`
	SourceSceneID        string    `gorm:"column:source_scene_id"`
	TemplateName         string    `gorm:"column:template_name"`
	Title                string    `gorm:"column:title"`
	Content              string    `gorm:"column:content"`
	SourceStatus         string    `gorm:"column:source_status"`
	IsQualified          bool      `gorm:"column:is_qualified"`
	IsPublished          bool      `gorm:"column:is_published"`
	VideoObjectPath      *string   `gorm:"column:video_object_path"`
	VideoSHA256          *string   `gorm:"column:video_sha256"`
	ThumbnailObjectPath  *string   `gorm:"column:thumbnail_object_path"`
	ThumbnailSHA256      *string   `gorm:"column:thumbnail_sha256"`
	ManualClipObjectPath *string   `gorm:"column:manual_clip_object_path"`
	ManualClipSHA256     *string   `gorm:"column:manual_clip_sha256"`
	SourceCreatedAt      time.Time `gorm:"column:source_created_at"`
	SourceUpdatedAt      time.Time `gorm:"column:source_updated_at"`
	ImportedAt           time.Time `gorm:"column:imported_at"`
}

func (pushRow) TableName() string { return "media_workflow.historical_video_pushes" }

type memberRow struct {
	PushID        string `gorm:"column:push_id;primaryKey"`
	Ordinal       int    `gorm:"column:ordinal;primaryKey"`
	SourceVideoID string `gorm:"column:source_video_id"`
	JobID         string `gorm:"column:job_id"`
}

func (memberRow) TableName() string { return "media_workflow.historical_video_push_members" }

func (r *Repo) Save(ctx context.Context, push *publication.HistoricalVideoPush) error {
	row := toRow(push)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for _, member := range push.Members {
			if err := tx.Create(&memberRow{PushID: push.ID, Ordinal: member.Ordinal, SourceVideoID: member.SourceVideoID, JobID: member.JobID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repo) GetBySource(ctx context.Context, workspaceID, sourceSystem, sourceRecordID string) (*publication.HistoricalVideoPush, error) {
	var row pushRow
	if err := r.db.WithContext(ctx).Where("workspace_id = ? AND source_system = ? AND source_record_id = ?", workspaceID, sourceSystem, sourceRecordID).First(&row).Error; err != nil {
		return nil, err
	}
	return r.withMembers(ctx, row)
}

func (r *Repo) Get(ctx context.Context, id string) (*publication.HistoricalVideoPush, error) {
	var row pushRow
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return r.withMembers(ctx, row)
}

func (r *Repo) List(ctx context.Context, workspaceID string, page, pageSize int) ([]*publication.HistoricalVideoPush, int64, error) {
	query := r.db.WithContext(ctx).Model(&pushRow{}).Where("workspace_id = ?", workspaceID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []pushRow
	if err := query.Order("source_created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*publication.HistoricalVideoPush, 0, len(rows))
	for _, row := range rows {
		item, err := r.withMembers(ctx, row)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

func (r *Repo) withMembers(ctx context.Context, row pushRow) (*publication.HistoricalVideoPush, error) {
	var members []memberRow
	if err := r.db.WithContext(ctx).Where("push_id = ?", row.ID).Order("ordinal ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	push := fromRow(row)
	for _, member := range members {
		push.Members = append(push.Members, publication.Member{Ordinal: member.Ordinal, SourceVideoID: member.SourceVideoID, JobID: member.JobID})
	}
	return push, nil
}

func toRow(push *publication.HistoricalVideoPush) pushRow {
	return pushRow{
		ID: push.ID, OrganizationID: push.OrganizationID, WorkspaceID: push.WorkspaceID, OwnerUserID: push.OwnerUserID,
		SourceSystem: push.SourceSystem, SourceRecordID: push.SourceRecordID, SourceDigest: push.SourceDigest,
		ProductID: push.ProductID, VirtualIPID: push.VirtualIPID, SourceSceneID: push.SourceSceneID,
		TemplateName: push.TemplateName, Title: push.Title, Content: push.Content, SourceStatus: push.SourceStatus,
		IsQualified: push.IsQualified, IsPublished: push.IsPublished,
		VideoObjectPath: nullable(push.Video.ObjectPath), VideoSHA256: nullable(push.Video.SHA256),
		ThumbnailObjectPath: nullable(push.Thumbnail.ObjectPath), ThumbnailSHA256: nullable(push.Thumbnail.SHA256),
		ManualClipObjectPath: nullable(push.ManualClip.ObjectPath), ManualClipSHA256: nullable(push.ManualClip.SHA256),
		SourceCreatedAt: push.SourceCreatedAt, SourceUpdatedAt: push.SourceUpdatedAt, ImportedAt: push.ImportedAt,
	}
}

func fromRow(row pushRow) *publication.HistoricalVideoPush {
	return &publication.HistoricalVideoPush{
		ID: row.ID, OrganizationID: row.OrganizationID, WorkspaceID: row.WorkspaceID, OwnerUserID: row.OwnerUserID,
		SourceSystem: row.SourceSystem, SourceRecordID: row.SourceRecordID, SourceDigest: row.SourceDigest,
		ProductID: row.ProductID, VirtualIPID: row.VirtualIPID, SourceSceneID: row.SourceSceneID,
		TemplateName: row.TemplateName, Title: row.Title, Content: row.Content, SourceStatus: row.SourceStatus,
		IsQualified: row.IsQualified, IsPublished: row.IsPublished,
		Video:           publication.MediaObject{ObjectPath: deref(row.VideoObjectPath), SHA256: deref(row.VideoSHA256)},
		Thumbnail:       publication.MediaObject{ObjectPath: deref(row.ThumbnailObjectPath), SHA256: deref(row.ThumbnailSHA256)},
		ManualClip:      publication.MediaObject{ObjectPath: deref(row.ManualClipObjectPath), SHA256: deref(row.ManualClipSHA256)},
		SourceCreatedAt: row.SourceCreatedAt, SourceUpdatedAt: row.SourceUpdatedAt, ImportedAt: row.ImportedAt,
	}
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
