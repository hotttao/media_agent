package container_workload

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_runtime_controller/container_workload"
	contract "media_agent/hertz_infra/agentworkload"
	native "media_agent/media_runtime_controller/docker"
	"strconv"
)

var application *native.Service

func Configure(s *native.Service) { application = s }
func EnsureContainer(ctx context.Context, c *app.RequestContext) {
	if application == nil {
		c.AbortWithStatus(503)
		return
	}
	var q pb.ContainerRequest
	if c.BindAndValidate(&q) != nil {
		c.AbortWithStatus(400)
		return
	}
	spec, e := contract.Decode(json.RawMessage(q.SpecJson))
	if e != nil || spec.WorkspaceID != c.Param("workspace_id") {
		c.AbortWithStatus(422)
		return
	}
	v, e := application.Ensure(ctx, native.State{Spec: spec, Version: q.TargetVersion, Operation: q.DrainOperation, PendingReplicas: q.PendingReplicas, PendingVersion: q.PendingVersion}, q.ExpectedRevision, "", q.Observation)
	if e != nil {
		status := 503
		switch {
		case errors.Is(e, native.ErrNotFound):
			status = 404
		case errors.Is(e, native.ErrConflict):
			status = 409
		case errors.Is(e, native.ErrInvalid):
			status = 422
		}
		c.JSON(status, map[string]string{"error": "container_request_failed"})
		return
	}
	body, _ := json.Marshal(v.State.Spec)
	c.JSON(200, &pb.ContainerResponse{ResourceUid: v.Container.ID, ResourceGeneration: native.Generation(v.Container), Running: v.Container.State.Running, Ready: native.Ready(v.Container), TargetVersion: v.State.Version, SpecJson: string(body), Revision: strconv.FormatInt(v.State.Revision, 10), DrainOperation: v.State.Operation, PendingReplicas: v.State.PendingReplicas, PendingVersion: v.State.PendingVersion})
}
