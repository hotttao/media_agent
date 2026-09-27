package generated_asset

import "time"

// Visibility 产物可见性（同 material）。
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityTeam    Visibility = "team"
	VisibilityPublic  Visibility = "public"
)

// GeneratedAsset workflow 产物实体（asset_kind.resource_type=ASSET）。
// 持生成记录反向链接（job_id / step_run_id / template_id / template_version），
// 详细生成上下文在 media_workflow.step_runs，本表只持链接。
// source_virtual_ip_id / source_product_id：生成时的输入容器（可达性 + 编排层 ListBySource 用）。
type GeneratedAsset struct {
	ID                string
	OrganizationID    string
	Scope             string
	Name              string
	AssetKind         string // resource_type=ASSET
	URL               string
	Content           string // TEXT 产物
	OwnerUserID       string // = job.submitted_by
	Visibility        Visibility
	TeamID            string
	JobID             string
	StepRunID         string
	TemplateID        string
	TemplateVersion   int
	SourceVirtualIPID string
	SourceProductID   string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
