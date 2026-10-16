package client

import (
	"context"
	"encoding/json"
	"github.com/cloudwego/hertz/pkg/protocol"
	sdk "media_agent/hertz_gen/media_runtime_controller/container_workload_service"
	pb "media_agent/hertz_gen/model/media_runtime_controller/container_workload"
	contract "media_agent/hertz_infra/agentworkload"
	"net/http"
)

type DockerWorkloads struct {
	EnvironmentID, BaseURL, ServiceToken string
	Bootstrap                            func(context.Context, contract.Spec) (string, error)
}

func (d DockerWorkloads) request(ctx context.Context, s *NativeWorkload, observe bool) (*pb.ContainerResponse, error) {
	if s.Spec.Driver != "docker" || s.Spec.EnvironmentID != d.EnvironmentID || d.ServiceToken == "" {
		return nil, ErrWorkloadConflict
	}
	if !observe && (s.Spec.Replicas != 1 || s.Operation != "" || s.PendingReplicas != 0 || s.PendingVersion != 0) {
		return nil, ErrWorkloadNotImplemented
	}
	observedStatus := 0
	api, e := sdk.NewContainerWorkloadServiceClient(d.BaseURL,
		sdk.WithHeader(http.Header{"X-Runtime-Creation-Credential": {d.ServiceToken}}),
		// Generated clients discard rawResponse on error. Capture the native
		// HTTP status before that boundary; never parse an error string/body.
		sdk.WithResponseResultDecider(func(status int, _ *protocol.Response) bool {
			observedStatus = status
			return status < 200 || status >= 400
		}))
	if e != nil {
		return nil, e
	}
	bootstrap := ""
	if !observe && d.Bootstrap != nil {
		bootstrap, e = d.Bootstrap(ctx, s.Spec)
		if e != nil {
			return nil, e
		}
	}
	body, _ := json.Marshal(s.Spec)
	v, raw, e := api.EnsureContainer(ctx, &pb.ContainerRequest{WorkspaceId: s.Spec.WorkspaceID, SpecJson: string(body), TargetVersion: s.Version, Observation: observe, ExpectedRevision: s.Revision, AgentApiToken: bootstrap})
	if raw != nil {
		observedStatus = raw.StatusCode()
	}
	switch observedStatus {
	case 404:
		return nil, ErrWorkloadAbsent
	case 409:
		return nil, ErrWorkloadConflict
	case 501:
		return nil, ErrWorkloadNotImplemented
	}
	if observedStatus >= 400 {
		return nil, &APIError{StatusCode: observedStatus, AgentType: "runtime_creation", Status: http.StatusText(observedStatus)}
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
	return &NativeWorkload{ReadyReplicas: ready, Settled: v.Running == (actual.Replicas == 1), Spec: actual, Revision: v.Revision, Version: v.TargetVersion}, nil
}
func (d DockerWorkloads) Save(ctx context.Context, s *NativeWorkload) error {
	_, e := d.request(ctx, s, false)
	return e
}
