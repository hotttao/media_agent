package client

import (
	"context"
	"encoding/json"
	sdk "media_agent/hertz_gen/media_runtime_controller/container_workload_service"
	pb "media_agent/hertz_gen/model/media_runtime_controller/container_workload"
	contract "media_agent/hertz_infra/agentworkload"
	"net/http"
)

type DockerWorkloads struct{ EnvironmentID, BaseURL, ServiceToken string }

func (d DockerWorkloads) request(ctx context.Context, s *NativeWorkload, observe bool) (*pb.ContainerResponse, error) {
	if s.Spec.Driver != "docker" || s.Spec.EnvironmentID != d.EnvironmentID || s.Spec.Replicas > 1 || d.ServiceToken == "" {
		return nil, ErrWorkloadConflict
	}
	api, e := sdk.NewContainerWorkloadServiceClient(d.BaseURL, sdk.WithHeader(http.Header{"X-Runtime-Creation-Credential": {d.ServiceToken}}))
	if e != nil {
		return nil, e
	}
	body, _ := json.Marshal(s.Spec)
	v, raw, e := api.EnsureContainer(ctx, &pb.ContainerRequest{WorkspaceId: s.Spec.WorkspaceID, SpecJson: string(body), TargetVersion: s.Version, Observation: observe, ExpectedRevision: s.Revision, DrainOperation: s.Operation, PendingReplicas: s.PendingReplicas, PendingVersion: s.PendingVersion})
	if raw != nil {
		switch raw.StatusCode() {
		case 404:
			return nil, ErrWorkloadAbsent
		case 409:
			return nil, ErrWorkloadConflict
		}
	}
	return v, e
}
func (d DockerWorkloads) Read(ctx context.Context, s contract.Spec) (*NativeWorkload, error) {
	v, e := d.request(ctx, &NativeWorkload{Spec: s}, true)
	if e != nil {
		return nil, e
	}
	actual, e := contract.Decode(json.RawMessage(v.SpecJson))
	if e != nil {
		return nil, e
	}
	if actual.EnvironmentID != s.EnvironmentID || actual.WorkspaceID != s.WorkspaceID || actual.AgentConfigID != s.AgentConfigID {
		return nil, ErrWorkloadConflict
	}
	ready := int32(0)
	if v.Ready {
		ready = 1
	}
	return &NativeWorkload{ReadyReplicas: ready, Settled: v.Running == (actual.Replicas == 1), Spec: actual, Revision: v.Revision, Version: v.TargetVersion, Operation: v.DrainOperation, PendingReplicas: v.PendingReplicas, PendingVersion: v.PendingVersion}, nil
}
func (d DockerWorkloads) Save(ctx context.Context, s *NativeWorkload) error {
	_, e := d.request(ctx, s, false)
	return e
}
