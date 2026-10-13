package client

import (
	"context"
	"encoding/json"
	"errors"
	sdk "media_agent/hertz_gen/media_control/agent_workload_service"
	pb "media_agent/hertz_gen/model/media_control/agent_workload"
	contract "media_agent/hertz_infra/agentworkload"
	"net/http"
)

type WorkloadTarget struct {
	Spec        contract.Spec
	Version     int64
	OwnerUserID string
}
type NativeWorkload struct {
	Settled         bool
	ReadyReplicas   int32
	Spec            contract.Spec
	Revision        string
	Version         int64
	PendingReplicas int32
	PendingVersion  int64
	Operation       string
}

var ErrWorkloadAbsent = errors.New("workload resource absent")
var ErrWorkloadConflict = errors.New("workload resource conflict")
var ErrWorkloadNotImplemented = errors.New("workload operation is not implemented")

type WorkloadControl struct{ BaseURL, ServiceToken string }

func target(v *pb.Target) (WorkloadTarget, error) {
	s, e := contract.Decode(json.RawMessage(v.SpecJson))
	if e != nil {
		return WorkloadTarget{}, e
	}
	if s.WorkspaceID != v.WorkspaceId || s.AgentConfigID != v.AgentConfigId || s.EnvironmentID != v.EnvironmentId || v.Version < 1 || v.Replicas < 0 || v.Replicas > 64 || v.OwnerUserId == "" {
		return WorkloadTarget{}, ErrNativeIdentity
	}
	s.Replicas = v.Replicas
	return WorkloadTarget{Spec: s, Version: v.Version, OwnerUserID: v.OwnerUserId}, nil
}
func (c WorkloadControl) Ensure(ctx context.Context, user, ws string) (WorkloadTarget, error) {
	api, e := sdk.NewAgentWorkloadServiceClient(c.BaseURL, sdk.WithHeader(http.Header{"X-Control-Service-Token": {c.ServiceToken}, "X-Control-Acting-User-ID": {user}}))
	if e != nil {
		return WorkloadTarget{}, e
	}
	v, _, e := api.EnsureWorkloadTarget(ctx, &pb.WorkloadQuery{WorkspaceId: ws})
	if e != nil {
		return WorkloadTarget{}, e
	}
	return target(v)
}
func (c WorkloadControl) List(ctx context.Context, after string) ([]WorkloadTarget, string, error) {
	api, e := sdk.NewAgentWorkloadServiceClient(c.BaseURL, sdk.WithHeader(http.Header{"X-Control-Service-Token": {c.ServiceToken}, "X-Control-Acting-User-ID": {"runtime-reconciler"}, "X-Control-Acting-Role": {"system"}}))
	if e != nil {
		return nil, "", e
	}
	v, _, e := api.ListWorkloadTargets(ctx, &pb.WorkloadQuery{AfterId: after})
	if e != nil {
		return nil, "", e
	}
	out := []WorkloadTarget{}
	for _, row := range v.Targets {
		t, e := target(row)
		if e != nil {
			return nil, "", e
		}
		out = append(out, t)
	}
	return out, v.NextAfterId, nil
}
