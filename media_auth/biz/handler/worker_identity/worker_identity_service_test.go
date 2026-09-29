package worker_identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	api "media_agent/media_auth/biz/domain/api_token"
	worker "media_agent/media_auth/biz/domain/worker_identity"
	service "media_agent/media_auth/biz/service/api_token"
)

func TestWorkerErrorMappingDoesNotExposeCredentialsOrInfrastructure(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{worker.ErrInvalid, 400}, {service.ErrInvalidRequest, 400}, {worker.ErrForbidden, 403}, {service.ErrForbidden, 403},
		{worker.ErrNotFound, 404}, {api.ErrNotFound, 404}, {worker.ErrConflict, 409}, {api.ErrConflict, 409},
		{service.ErrInactive, 503}, {errors.New("Talos SQL failure secret=synthetic-sensitive-value"), 500},
	} {
		ctx := ut.CreateUtRequestContext("POST", "/api/worker-identities", nil)
		writeError(ctx, tc.err)
		if ctx.Response.StatusCode() != tc.status {
			t.Fatal("error HTTP mapping mismatch")
		}
		if body := string(ctx.Response.Body()); strings.Contains(body, "synthetic-sensitive") || strings.Contains(body, "SQL") || strings.Contains(body, "Talos") {
			t.Fatal("infrastructure error leaked")
		}
	}
}

func TestWorkerManagementWithoutVerifiedOwnerFailsBeforeBinding(t *testing.T) {
	ctx := ut.CreateUtRequestContext("POST", "/api/worker-identities", nil)
	CreateWorkerIdentity(t.Context(), ctx)
	if ctx.Response.StatusCode() != 401 {
		t.Fatal("unauthenticated identity creation accepted")
	}
	ctx = ut.CreateUtRequestContext("POST", "/api/worker-identities/id/api-tokens", nil)
	CreateWorkerApiToken(t.Context(), ctx)
	if ctx.Response.StatusCode() != 401 {
		t.Fatal("unauthenticated credential creation accepted")
	}
}
