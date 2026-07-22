package shared

import "context"

// BindingEnder 失效 owner 的 material_bindings（置 bindings_end_at = now）。
//
// 由 material service 实现（EndBindingsForOwner），注入到 virtual_ip / product service，
// 在 owner 软删时调用。跨 domain 写用接口注入，避免 service 跨层 import
// （参照 OwnerScopeLookup 模式：material 调 vip/product 读 scope；vip/product 调 material 写 binding）。
type BindingEnder interface {
	EndBindingsForOwner(ctx context.Context, ownerKind, ownerID string) error
}
