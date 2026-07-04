package generated_asset

import domain "media_agent/media_materials/biz/domain/generated_asset"

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
	TemplateID        string
	TemplateVersion   int
	SourceVirtualIPID string
	SourceProductID   string
}

type ListQuery = domain.ListQuery
