package agent_workload

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_control/agent_workload"
	d "media_agent/media_control/biz/domain/runtime_environment"
	"media_agent/media_control/biz/middleware"
	workflow "media_agent/media_control/biz/workflow/workload_target"
)

var application *workflow.Flow

func Configure(f *workflow.Flow) { application = f }
func response(v *d.WorkloadTarget) *pb.Target {
	spec := map[string]any{}
	_ = json.Unmarshal(v.Spec, &spec)
	spec["replicas"] = v.Replicas
	body, _ := json.Marshal(spec)
	return &pb.Target{WorkspaceId: v.WorkspaceID, AgentConfigId: v.AgentConfigID, EnvironmentId: v.EnvironmentID, OwnerUserId: v.OwnerUserID, Version: v.Version, Replicas: v.Replicas, SpecJson: string(body)}
}
func failure(c *app.RequestContext, e error) {
	status := 503
	switch {
	case errors.Is(e, workflow.ErrForbidden):
		status = 403
	case errors.Is(e, workflow.ErrInvalid):
		status = 422
	case errors.Is(e, d.ErrNotFound):
		status = 404
	case errors.Is(e, d.ErrConflict):
		status = 409
	}
	c.JSON(status, map[string]string{"error": "workload_target_failed"})
}
func query(ctx context.Context, c *app.RequestContext, ensure bool) {
	if application == nil {
		failure(c, errors.New("disabled"))
		return
	}
	var q pb.WorkloadQuery
	if c.BindAndValidate(&q) != nil {
		c.JSON(400, map[string]string{"error": "invalid_request"})
		return
	}
	var v *d.WorkloadTarget
	var e error
	if ensure {
		v, e = application.Ensure(ctx, c.Param("workspace_id"), middleware.UserID(ctx))
	} else {
		v, e = application.Get(ctx, c.Param("workspace_id"), middleware.UserID(ctx))
	}
	if e != nil {
		failure(c, e)
		return
	}
	c.JSON(200, response(v))
}
func EnsureWorkloadTarget(ctx context.Context, c *app.RequestContext) { query(ctx, c, true) }
func GetWorkloadTarget(ctx context.Context, c *app.RequestContext)    { query(ctx, c, false) }
func UpdateWorkloadTarget(ctx context.Context, c *app.RequestContext) {
	if application == nil {
		failure(c, errors.New("disabled"))
		return
	}
	var q pb.UpdateTarget
	if c.BindAndValidate(&q) != nil {
		c.JSON(400, map[string]string{"error": "invalid_request"})
		return
	}
	v, e := application.Update(ctx, c.Param("workspace_id"), middleware.UserID(ctx), q.IdempotencyKey, q.Replicas, q.ExpectedVersion)
	if e != nil {
		failure(c, e)
		return
	}
	c.JSON(200, response(v))
}
func ListWorkloadTargets(ctx context.Context, c *app.RequestContext) {
	if application == nil {
		failure(c, errors.New("disabled"))
		return
	}
	var q pb.WorkloadQuery
	if c.BindAndValidate(&q) != nil {
		c.JSON(400, map[string]string{"error": "invalid_request"})
		return
	}
	rows, e := application.Service.ListWorkloads(ctx, q.AfterId)
	if e != nil {
		failure(c, e)
		return
	}
	out := &pb.Targets{}
	for _, v := range rows {
		out.Targets = append(out.Targets, response(&v))
	}
	if len(rows) == 100 {
		out.NextAfterId = rows[len(rows)-1].WorkspaceID
	}
	c.JSON(200, out)
}
