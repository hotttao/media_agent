package generated_asset

import "context"

type Repo interface {
	Save(ctx context.Context, a *GeneratedAsset) error
	Get(ctx context.Context, id string) (*GeneratedAsset, error)
	List(ctx context.Context, q ListQuery) ([]*GeneratedAsset, int, error)
	ListBySource(ctx context.Context, sourceKind, sourceID string) ([]*GeneratedAsset, error)
	Delete(ctx context.Context, id string) error
}

type ListQuery struct {
	Scope        string
	AssetKind    string
	Owner        string
	JobID        string
	SourceVipID  string
	SourceProdID string
	Limit        int
	Offset       int
}
