// Package reachability 是跨 domain 可达性编排（biz/workflow）。
//
// 编排 material / generated_asset / virtual_ip / product service 的只读查询加载事实，
// 调 biz/policy/reachability 做纯决策。归 biz/workflow（跨 domain 编排唯一合法入口，ADR 0006）。
// 供 caller（BFF）组装 JobInput bundle 时调用；media_workflow 不调（bundle 模式）。
package reachability

import (
	"context"
	"fmt"

	policy "media_agent/media_materials/biz/policy/reachability"
	assetkindsvc "media_agent/media_materials/biz/service/asset_kind"
	genassetsvc "media_agent/media_materials/biz/service/generated_asset"
	materialsvc "media_agent/media_materials/biz/service/material"
	productsvc "media_agent/media_materials/biz/service/product"
	virtualipsvc "media_agent/media_materials/biz/service/virtual_ip"
)

// Ref 跨服务引用（resource_type + id）。
type Ref struct {
	ResourceType string
	ID           string
}

// Blocked 不可达项。
type Blocked struct {
	Ref    Ref
	Reason string
}

// Flow reachability workflow。
type Flow struct {
	materialSvc  *materialsvc.Service
	genAssetSvc  *genassetsvc.Service
	vipSvc       *virtualipsvc.Service
	productSvc   *productsvc.Service
	assetKindSvc *assetkindsvc.Service
}

func NewFlow(
	materialSvc *materialsvc.Service,
	genAssetSvc *genassetsvc.Service,
	vipSvc *virtualipsvc.Service,
	productSvc *productsvc.Service,
	assetKindSvc *assetkindsvc.Service,
) *Flow {
	return &Flow{
		materialSvc: materialSvc, genAssetSvc: genAssetSvc,
		vipSvc: vipSvc, productSvc: productSvc, assetKindSvc: assetKindSvc,
	}
}

// AssetReachability 批量校验 refs 在 (userID, workflowScope) 下可达。
// material / generated_asset：scope ∧ visibility ∧ 容器可达。
func (f *Flow) AssetReachability(ctx context.Context, userID, workflowScope string, refs []Ref) (map[string]bool, []Blocked) {
	reachable := map[string]bool{}
	var blocked []Blocked
	for _, r := range refs {
		key := r.ResourceType + ":" + r.ID
		ok, reason := f.canReachAsset(ctx, r, userID, workflowScope)
		if ok {
			reachable[key] = true
		} else {
			blocked = append(blocked, Blocked{Ref: r, Reason: string(reason)})
		}
	}
	return reachable, blocked
}

func (f *Flow) canReachAsset(ctx context.Context, r Ref, userID, workflowScope string) (bool, policy.Reason) {
	switch r.ResourceType {
	case "MATERIAL":
		// materialSvc.Get 带 callerID/teams 可见性校验；不可见 -> err（视为 NOT_VISIBLE/NOT_FOUND）。
		m, err := f.materialSvc.Get(ctx, r.ID, userID, nil)
		if err != nil {
			return false, policy.ReasonNotFound
		}
		// TODO: 容器可达需查 material 的 binding owner（ListBindingsForMaterial，repo 当前无此方法）。
		// 暂按 product 路径假设（product binding 存在则 scope 匹配即达）。
		fact := policy.AssetFact{
			ResourceType: "MATERIAL", Scope: m.Scope, Visibility: string(m.Visibility),
			OwnerUserID: m.OwnerUserID, TeamID: m.TeamID, ContainerProduct: true,
		}
		return policy.CanReachAsset(fact, userID, workflowScope, nil)
	case "ASSET":
		a, err := f.genAssetSvc.Get(ctx, r.ID)
		if err != nil {
			return false, policy.ReasonNotFound
		}
		// 容器可达：source_product_id 非空（product 路径）；source_virtual_ip_id 对 user 可见（vip 路径，TODO 查 vip 可见）。
		fact := policy.AssetFact{
			ResourceType: "ASSET", Scope: a.Scope, Visibility: string(a.Visibility),
			OwnerUserID: a.OwnerUserID, TeamID: a.TeamID, ContainerProduct: a.SourceProductID != "",
		}
		return policy.CanReachAsset(fact, userID, workflowScope, nil)
	}
	return false, policy.ReasonNotFound
}

// CheckProductAccessible 校验 product.scope == workflowScope（无 user）。
func (f *Flow) CheckProductAccessible(ctx context.Context, workflowScope, productID string) (bool, string) {
	p, err := f.productSvc.Get(ctx, productID)
	if err != nil {
		return false, string(policy.ReasonNotFound)
	}
	ok, reason := policy.CanReachProduct(p.Scope, workflowScope)
	return ok, string(reason)
}

// CheckVirtualIPAccessible 校验 vip.scope == workflowScope ∧ MyPermission(user,vip) ≥ viewer。
// vipSvc.Get 带可见性校验（canRead），不可见 -> err。
func (f *Flow) CheckVirtualIPAccessible(ctx context.Context, userID, workflowScope, vipID string) (bool, string) {
	v, err := f.vipSvc.Get(ctx, vipID, userID, nil)
	if err != nil {
		return false, string(policy.ReasonNotFound)
	}
	ok, reason := policy.CanReachVip(v.Scope, workflowScope, true) // Get 成功即 ≥ viewer
	return ok, string(reason)
}

// ResolveVirtualIpForReachability 返回 user 对哪些 vip 至少 viewer。
func (f *Flow) ResolveVirtualIpForReachability(ctx context.Context, userID string, vipIDs []string) []string {
	visible := make([]string, 0, len(vipIDs))
	for _, id := range vipIDs {
		if _, err := f.vipSvc.Get(ctx, id, userID, nil); err == nil {
			visible = append(visible, id)
		}
	}
	return visible
}

// ResolveAssetByKind 按 (ownerKind, ownerID, assetKind) 取唯一 asset。
// 按 asset_kind.resource_type 分派：MATERIAL -> material_bindings 找 owner 下的 material；
// ASSET -> generated_assets 找 owner 下的 generated_asset。
func (f *Flow) ResolveAssetByKind(ctx context.Context, ownerKind, ownerID, assetKind string) (resourceType, assetID string, err error) {
	rt, _, _, lookupErr := f.assetKindSvc.Lookup(ctx, assetKind)
	if lookupErr != nil {
		return "", "", fmt.Errorf("asset_kind %q: %w", assetKind, lookupErr)
	}
	switch rt {
	case "MATERIAL":
		materials, mErr := f.materialSvc.ListByOwner(ctx, ownerKind, ownerID)
		if mErr != nil {
			return "", "", fmt.Errorf("list materials by owner: %w", mErr)
		}
		var matched []string
		for _, m := range materials {
			if m.AssetKind == assetKind {
				matched = append(matched, m.ID)
			}
		}
		if len(matched) == 0 {
			return rt, "", fmt.Errorf("no material of kind %q under %s/%s", assetKind, ownerKind, ownerID)
		}
		if len(matched) > 1 {
			return rt, "", fmt.Errorf("ambiguous: %d materials of kind %q under %s/%s", len(matched), assetKind, ownerKind, ownerID)
		}
		return rt, matched[0], nil
	case "ASSET":
		// generated_asset 按 source 查：owner_kind=virtual_ip -> source_virtual_ip_id; product -> source_product_id。
		sourceKind := "virtual_ip"
		if ownerKind == "product" {
			sourceKind = "product"
		}
		assets, aErr := f.genAssetSvc.ListBySource(ctx, sourceKind, ownerID)
		if aErr != nil {
			return "", "", fmt.Errorf("list generated_assets by source: %w", aErr)
		}
		var matched []string
		for _, a := range assets {
			if a.AssetKind == assetKind {
				matched = append(matched, a.ID)
			}
		}
		if len(matched) == 0 {
			return rt, "", fmt.Errorf("no generated_asset of kind %q under %s/%s", assetKind, ownerKind, ownerID)
		}
		if len(matched) > 1 {
			return rt, "", fmt.Errorf("ambiguous: %d generated_assets of kind %q under %s/%s", len(matched), assetKind, ownerKind, ownerID)
		}
		return rt, matched[0], nil
	}
	return "", "", fmt.Errorf("unsupported resource_type %q for ResolveAssetByKind", rt)
}

// AssetRead 按 (resource_type, id) 取 url + content + metadata（不返回二进制）。
func (f *Flow) AssetRead(ctx context.Context, resourceType, id string) (url, content, metadata string, err error) {
	switch resourceType {
	case "MATERIAL":
		m, e := f.materialSvc.Get(ctx, id, "", nil)
		if e != nil {
			return "", "", "", fmt.Errorf("material %q: %w", id, e)
		}
		return m.URL, m.Content, "", nil
	case "ASSET":
		a, e := f.genAssetSvc.Get(ctx, id)
		if e != nil {
			return "", "", "", fmt.Errorf("generated_asset %q: %w", id, e)
		}
		return a.URL, a.Content, "", nil
	}
	return "", "", "", fmt.Errorf("unsupported resource_type %q for AssetRead", resourceType)
}
