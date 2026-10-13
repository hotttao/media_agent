package runtime_instance_identity

import (
	"context"
	"errors"
	"fmt"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_runtime_controller/runtime_instance_identity"
	contract "media_agent/hertz_infra/agentworkload"
	native "media_agent/media_runtime_controller/docker"
)

var application *native.Service

func Configure(s *native.Service) { application = s }
func ResolveContainerIdentity(ctx context.Context, c *app.RequestContext) {
	if application == nil {
		c.AbortWithStatus(503)
		return
	}
	var q pb.IdentityRequest
	if c.BindAndValidate(&q) != nil {
		c.AbortWithStatus(400)
		return
	}
	v, terminated, e := application.Identity(ctx, c.Param("workspace_id"), q.AgentConfigId, q.ResourceUid, q.ResourceGeneration, q.BootstrapProof, q.Observation)
	if e != nil {
		status := 503
		if errors.Is(e, native.ErrNotFound) {
			status = 404
		}
		if errors.Is(e, native.ErrConflict) {
			status = 403
		}
		c.JSON(status, map[string]string{"error": "native_identity_failed"})
		return
	}
	s := v.State.Spec
	c.JSON(200, &pb.IdentityResponse{EnvironmentId: s.EnvironmentID, WorkspaceId: s.WorkspaceID, AgentConfigId: s.AgentConfigID, ResourceUid: v.Container.ID, ResourceGeneration: native.Generation(v.Container), Endpoint: fmt.Sprintf("http://%s:%d", contract.Name(s.EnvironmentID, s.WorkspaceID), s.Port), Eligible: s.Replicas == 1, Ready: native.Ready(v.Container) && s.Replicas == 1, Running: v.Container.State.Running, Terminated: terminated, AgentType: s.AgentType})
}
