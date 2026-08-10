// Package reachability 是可达性纯决策逻辑（无副作用、不查 DB）。
//
// 归 biz/policy（ADR 0006）：接收 workflow 加载好的事实，返回决策结果。
// 规则定义见 docs/ddd/media_materials/workflow/0001-reachability/reachability.md。
package reachability

// Reason 不可达原因。
type Reason string

const (
	ReasonScopeMismatch        Reason = "SCOPE_MISMATCH"
	ReasonNotVisible           Reason = "NOT_VISIBLE"
	ReasonContainerUnreachable Reason = "CONTAINER_UNREACHABLE"
	ReasonNotFound             Reason = "NOT_FOUND"
)

// AssetFact workflow 加载好的 asset（material / generated_asset）事实。
type AssetFact struct {
	ResourceType string // MATERIAL / ASSET
	Scope        string
	Visibility   string // private / team / public
	OwnerUserID  string
	TeamID       string
	// 容器可达：
	//   material: binding owner_kind=virtual_ip 且 user 对该 vip 可见；或 owner_kind=product（仅 scope）。
	//   generated_asset: source_virtual_ip_id 对 user 可见；或 source_product_id 非空。
	ContainerVipVisible bool
	ContainerProduct    bool
}

// VisibilityRule 推导 asset 对 user 可见（public ∨ private(owner) ∨ team(user∈team)）。
// 对齐 docs/ddd/media_auth/CONTEXT.md#资源级-acl 的 private/team/public 三档。
func VisibilityRule(visibility, ownerUserID, userID, teamID string, userTeams []string) bool {
	switch visibility {
	case "public":
		return true
	case "private":
		return ownerUserID != "" && ownerUserID == userID
	case "team":
		if teamID == "" {
			return false
		}
		for _, t := range userTeams {
			if t == teamID {
				return true
			}
		}
	}
	return false
}

// CanReachAsset 推导 asset 在 workflowScope 下对 user 可达。
// can = scope_match ∧ visibility_rule ∧ container_reachable
// material / generated_asset 通用（容器可达由 workflow 推导后填入 AssetFact）。
func CanReachAsset(fact AssetFact, userID, workflowScope string, userTeams []string) (bool, Reason) {
	if fact.Scope != workflowScope {
		return false, ReasonScopeMismatch
	}
	if !VisibilityRule(fact.Visibility, fact.OwnerUserID, userID, fact.TeamID, userTeams) {
		return false, ReasonNotVisible
	}
	// 容器可达：vip 路径（对 user 可见）或 product 路径（仅 scope 匹配，无 user）。
	if !fact.ContainerVipVisible && !fact.ContainerProduct {
		return false, ReasonContainerUnreachable
	}
	return true, ""
}

// CanReachVip 推导 virtual_ip 输入可达（scope ∧ MyPermission(user,vip) ≥ viewer）。
// vipVisibleToUser 由 workflow 推导（owner ∨ public ∨ team ∨ grant ≥ viewer）。
func CanReachVip(vipScope, workflowScope string, vipVisibleToUser bool) (bool, Reason) {
	if vipScope != workflowScope {
		return false, ReasonScopeMismatch
	}
	if !vipVisibleToUser {
		return false, ReasonNotVisible
	}
	return true, ""
}

// CanReachProduct 推导 product 输入可达（仅 scope 匹配，product 无 user 概念）。
func CanReachProduct(productScope, workflowScope string) (bool, Reason) {
	if productScope != workflowScope {
		return false, ReasonScopeMismatch
	}
	return true, ""
}
