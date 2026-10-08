package template

import (
	"context"
	"media_agent/media_workflow/biz/shared/auth"
)

func resourceOrganization(ctx context.Context, requested string) string {
	if caller := auth.CallerFromContext(ctx); caller != nil && caller.OrganizationID != "" {
		if requested != "" && requested != caller.OrganizationID {
			return ""
		}
		return caller.OrganizationID
	}
	return requested
}
