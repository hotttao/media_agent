package workflow

import (
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	domain "media_agent/media_workflow/biz/domain/workflow"
	"media_agent/media_workflow/biz/shared"
)

func resourceError(c *app.RequestContext, err error) bool {
	if errors.Is(err, shared.ErrResourceAuthorityUnavailable) {
		c.String(503, "resource authorization unavailable")
		return true
	}
	if errors.Is(err, shared.ErrResourceAccessDenied) || errors.Is(err, domain.ErrWorkflowForbidden) {
		c.String(403, "resource access denied")
		return true
	}
	return false
}
