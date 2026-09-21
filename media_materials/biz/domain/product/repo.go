package product

import "context"

type Repo interface {
	Save(ctx context.Context, p *Product) error
	Get(ctx context.Context, id string) (*Product, error)
	List(ctx context.Context, q ListQuery) ([]*Product, int, error)
	Update(ctx context.Context, p *Product) error
	Delete(ctx context.Context, id string) error
}

type ListQuery struct {
	OrganizationID string
	Scope          string
	Name           string
	TargetAudience string
	Tag            string
	Status         string
	Limit          int
	Offset         int
}
