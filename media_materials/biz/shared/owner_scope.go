package shared

import (
	"context"
	"fmt"
)

// OwnerScopeLookup 复合实现：按 owner_kind 分派到 virtual_ip / product service。
// vip / product 是各自的 GetOwnerScope 方法（签名含 ownerKind 参数）。
type OwnerScopeLookup struct {
	vip     func(ctx context.Context, ownerKind, ownerID string) (string, error)
	product func(ctx context.Context, ownerKind, ownerID string) (string, error)
}

func NewOwnerScopeLookup(
	vip func(ctx context.Context, ownerKind, ownerID string) (string, error),
	product func(ctx context.Context, ownerKind, ownerID string) (string, error),
) *OwnerScopeLookup {
	return &OwnerScopeLookup{vip: vip, product: product}
}

func (o *OwnerScopeLookup) GetOwnerScope(ctx context.Context, ownerKind, ownerID string) (string, error) {
	switch ownerKind {
	case "virtual_ip":
		return o.vip(ctx, ownerKind, ownerID)
	case "product":
		return o.product(ctx, ownerKind, ownerID)
	}
	return "", fmt.Errorf("unknown owner_kind %q", ownerKind)
}
