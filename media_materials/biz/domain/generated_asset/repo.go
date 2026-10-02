package generated_asset

import "context"

type Repo interface {
	Save(ctx context.Context, a *GeneratedAsset) error
	Get(ctx context.Context, id string) (*GeneratedAsset, error)
	GetByStepRun(ctx context.Context, stepRunID string) (*GeneratedAsset, error)
	GetByLegacySource(ctx context.Context, sourceSystem, sourceRecordID string) (*GeneratedAsset, error)
	List(ctx context.Context, q ListQuery) ([]*GeneratedAsset, int, error)
	ListBySource(ctx context.Context, sourceKind, sourceID string) ([]*GeneratedAsset, error)
	Delete(ctx context.Context, id string) error
}

type ListQuery struct {
	OrganizationIDs []string
	CallerID        string
	CallerTeamIDs   []string
	Scope           string
	AssetKind       string
	Owner           string
	JobID           string
	SourceVipID     string
	SourceProdID    string
	Limit           int
	Offset          int
}
