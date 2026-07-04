package virtual_ip

import "context"

type Repo interface {
	Save(ctx context.Context, v *VirtualIp) error
	Get(ctx context.Context, id string) (*VirtualIp, error)
	List(ctx context.Context, q ListQuery) ([]*VirtualIp, int, error)
	Update(ctx context.Context, v *VirtualIp) error
	Delete(ctx context.Context, id string) error
}

type GrantRepo interface {
	AddGrant(ctx context.Context, g *Grant) error
	RemoveGrant(ctx context.Context, virtualIPID, teamID string) error
	ListGrants(ctx context.Context, virtualIPID string) ([]*Grant, error)
	ListByTeam(ctx context.Context, teamID string) ([]*Grant, error)
}

type ListQuery struct {
	Scope      string
	Name       string
	Status     string
	Visibility string
	Limit      int
	Offset     int
}
