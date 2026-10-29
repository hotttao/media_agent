package media_management

import (
	"context"
	"media_agent/hertz_infra/authn/mcpdelegation"
	policy "media_agent/media_worker/biz/policy/media_management"
	contract "media_agent/media_worker/biz/shared/media_management"
	"strings"
)

type Management struct{ API contract.API }

func (m Management) Execute(ctx context.Context, operation string, items []Item[contract.Params]) (BatchResult[map[string]any], error) {
	if m.API == nil || !policy.Known(operation) {
		return BatchResult[map[string]any]{}, ErrInvalidBatch
	}
	principal, ok := mcpdelegation.FromContext(ctx)
	if !ok || !mcpdelegation.ValidPrincipal(principal) {
		return BatchResult[map[string]any]{}, &Failure{Code: "unauthenticated", Message: "Verified caller required", Stop: true}
	}
	if policy.NeedsWorkspace(operation) && principal.WorkspaceID == "" {
		return BatchResult[map[string]any]{}, &Failure{Code: "workspace_required", Message: "Select a Workspace", Stop: true}
	}
	if strings.HasSuffix(operation, "_list") {
		total := 0
		for _, item := range items {
			total += item.Params.PageSize
		}
		if total > 200 {
			return BatchResult[map[string]any]{}, ErrInvalidBatch
		}
	}
	return (Batch[contract.Params, map[string]any]{ReadOnly: policy.ReadOnly(operation), Validate: func(p contract.Params) error { return policy.Validate(operation, p) }, DuplicateKey: func(p contract.Params) string { return policy.DuplicateKey(operation, p) }, Execute: func(ctx context.Context, p contract.Params) (map[string]any, error) {
		return m.API.Execute(ctx, operation, p)
	}}).Run(ctx, items)
}
