package historical_publication

import (
	"context"
	"time"
)

type MediaObject struct {
	ObjectPath string `json:"object_path"`
	SHA256     string `json:"sha256"`
	AccessURL  string `json:"access_url,omitempty"`
}

type Member struct {
	Ordinal       int    `json:"ordinal"`
	SourceVideoID string `json:"source_video_id"`
	JobID         string `json:"job_id"`
}

// HistoricalVideoPush is a frozen source fact, not a publishing command.
type HistoricalVideoPush struct {
	ID              string      `json:"id"`
	OrganizationID  string      `json:"organization_id"`
	WorkspaceID     string      `json:"workspace_id"`
	OwnerUserID     string      `json:"owner_user_id"`
	SourceSystem    string      `json:"source_system"`
	SourceRecordID  string      `json:"source_record_id"`
	SourceDigest    string      `json:"source_digest"`
	ProductID       string      `json:"product_id"`
	VirtualIPID     string      `json:"virtual_ip_id"`
	SourceSceneID   string      `json:"source_scene_id"`
	TemplateName    string      `json:"template_name"`
	Title           string      `json:"title"`
	Content         string      `json:"content"`
	SourceStatus    string      `json:"source_status"`
	IsQualified     bool        `json:"is_qualified"`
	IsPublished     bool        `json:"is_published"`
	Video           MediaObject `json:"video"`
	Thumbnail       MediaObject `json:"thumbnail"`
	ManualClip      MediaObject `json:"manual_clip"`
	SourceCreatedAt time.Time   `json:"source_created_at"`
	SourceUpdatedAt time.Time   `json:"source_updated_at"`
	ImportedAt      time.Time   `json:"imported_at"`
	Members         []Member    `json:"members"`
}

type Repo interface {
	GetBySource(ctx context.Context, workspaceID, sourceSystem, sourceRecordID string) (*HistoricalVideoPush, error)
	Get(ctx context.Context, id string) (*HistoricalVideoPush, error)
	List(ctx context.Context, workspaceID string, page, pageSize int) ([]*HistoricalVideoPush, int64, error)
	Save(ctx context.Context, push *HistoricalVideoPush) error
}
