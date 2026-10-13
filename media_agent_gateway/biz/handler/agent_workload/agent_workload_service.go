package agent_workload

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	pb "media_agent/hertz_gen/model/media_agent_gateway/agent_workload"
	contract "media_agent/hertz_infra/agentworkload"
	workflow "media_agent/media_agent_gateway/biz/workflow"
)

var application *workflow.WorkloadProvisioning

func Configure(f *workflow.WorkloadProvisioning) { application = f }
func handle(ctx context.Context, c *app.RequestContext, action string) {
	if application == nil {
		c.AbortWithStatus(503)
		return
	}
	user := string(c.GetHeader("X-Control-Acting-User-ID"))
	if user == "" {
		c.AbortWithStatus(403)
		return
	}
	var q pb.WorkloadRequest
	if c.BindAndValidate(&q) != nil {
		c.AbortWithStatus(400)
		return
	}
	ws := c.Param("workspace_id")
	var e error
	if action == "ensure" {
		e = application.Ensure(ctx, user, ws)
	}
	if action == "scale" {
		e = application.Scale(ctx, user, ws, q.IdempotencyKey, q.Replicas, q.ExpectedVersion)
	}
	pending := errors.Is(e, workflow.ErrRuntimeWaiting)
	if e != nil && !pending {
		status := 503
		code := "workload_operation_failed"
		if errors.Is(e, workflow.ErrWorkloadNotImplemented) {
			status, code = 501, "not_implemented"
		}
		if errors.Is(e, workflow.ErrWorkloadConflict) {
			status = 409
		}
		c.JSON(status, map[string]string{"error": code})
		return
	}
	v, e := application.View(ctx, user, ws)
	if e != nil {
		c.JSON(503, map[string]string{"error": "workload_view_unavailable"})
		return
	}
	s := v.Target.Spec
	out := &pb.WorkloadView{WorkspaceId: ws, EnvironmentId: s.EnvironmentID, AgentConfigId: s.AgentConfigID, ResourceName: contract.Name(s.EnvironmentID, ws), Namespace: s.Namespace, TargetVersion: v.Target.Version, DesiredReplicas: s.Replicas}
	if v.Native != nil {
		out.AppliedVersion = v.Native.Version
		out.CommittedReplicas = v.Native.Spec.Replicas
		out.ReadyReplicas = v.Native.ReadyReplicas
		out.DrainOperation = v.Native.Operation
	}
	status := 200
	if pending {
		status = 202
	}
	c.JSON(status, out)
}
func EnsureWorkload(ctx context.Context, c *app.RequestContext) { handle(ctx, c, "ensure") }
func GetWorkload(ctx context.Context, c *app.RequestContext)    { handle(ctx, c, "get") }
func ScaleWorkload(ctx context.Context, c *app.RequestContext)  { handle(ctx, c, "scale") }
