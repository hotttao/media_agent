package generated_asset

import (
	"time"

	domain "media_agent/media_materials/biz/domain/generated_asset"
)

// UpsertCommand 由 media_workflow execution 期调用，落产物 + 生成记录。
type UpsertCommand struct {
	Scope             string
	AssetKind         string // = template.output_asset_kind
	Name              string
	URL               string
	Content           string
	OwnerUserID       string // = job.submitted_by
	Visibility        string // 默认 private
	TeamID            string
	JobID             string
	StepRunID         string
	ResultKey         string // Immutable successful generation identity; empty only for legacy callers.
	TemplateID        string
	TemplateVersion   int
	SourceVirtualIPID string
	SourceProductID   string
}

type ListQuery = domain.ListQuery

// ImportCommand is accepted only through the migration-only internal API.
// ObjectPath already points at a verified object in our own storage.
type ImportCommand struct {
	OrganizationID    string
	Scope             string
	AssetKind         string
	ObjectPath        string
	OwnerUserID       string
	Name              string
	SourceSystem      string
	SourceRecordID    string
	SourceSHA256      string
	SourceCreatedAt   time.Time
	SourceVirtualIPID string
	SourceProductID   string
}
