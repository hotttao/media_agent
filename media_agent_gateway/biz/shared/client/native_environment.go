package client

import (
	"context"
	sdk "media_agent/hertz_gen/media_control/runtime_environment_service"
	dockerSDK "media_agent/hertz_gen/media_runtime_controller/runtime_instance_identity_service"
	pb "media_agent/hertz_gen/model/media_control/runtime_environment"
	dockerPB "media_agent/hertz_gen/model/media_runtime_controller/runtime_instance_identity"
	d "media_agent/media_agent_gateway/biz/domain/agent_instance"
	"net/http"
	"time"
)

type PlacementAuthority struct{ BaseURL, ServiceToken string }

func (c PlacementAuthority) Environment(ctx context.Context, user, workspace string) (string, error) {
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	api, err := sdk.NewRuntimeEnvironmentServiceClient(c.BaseURL, sdk.WithHeader(http.Header{"X-Control-Service-Token": {c.ServiceToken}, "X-Control-Acting-User-ID": {user}}))
	if err != nil {
		return "", ErrNativeUnavailable
	}
	value, _, err := api.GetPlacementStatus(call, &pb.GetPlacementStatusRequest{WorkspaceId: workspace})
	if err != nil {
		return "", ErrNativeUnavailable
	}
	if value.WorkspaceId != workspace || value.EnvironmentId == "" || value.Status != "placed" && value.Status != "ready" {
		return "", ErrNativeIdentity
	}
	return value.EnvironmentId, nil
}

type DockerIdentity struct{ EnvironmentID, BaseURL, ServiceToken string }

func (c DockerIdentity) request(ctx context.Context, l d.ResourceLocator, proof, expectedGeneration string, observation bool) (d.ResourceFacts, bool, error) {
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if l.EnvironmentID != c.EnvironmentID || l.ReplicaID != "0" || c.ServiceToken == "" {
		return d.ResourceFacts{}, false, ErrNativeIdentity
	}
	api, err := dockerSDK.NewRuntimeInstanceIdentityServiceClient(c.BaseURL, dockerSDK.WithHeader(http.Header{"X-Runtime-Creation-Credential": {c.ServiceToken}}))
	if err != nil {
		return d.ResourceFacts{}, false, ErrNativeUnavailable
	}
	v, _, err := api.ResolveContainerIdentity(call, &dockerPB.IdentityRequest{WorkspaceId: l.WorkspaceID, AgentConfigId: l.AgentConfigID, BootstrapProof: proof, ResourceUid: l.ResourceUID, ResourceGeneration: expectedGeneration, Observation: observation})
	if err != nil {
		return d.ResourceFacts{}, false, ErrNativeUnavailable
	}
	if v.EnvironmentId != l.EnvironmentID || v.WorkspaceId != l.WorkspaceID || v.AgentConfigId != l.AgentConfigID || v.ResourceUid == "" || v.ResourceGeneration == "" || v.Endpoint == "" {
		return d.ResourceFacts{}, false, ErrNativeIdentity
	}
	l.ResourceUID = v.ResourceUid
	return d.ResourceFacts{Locator: l, Generation: v.ResourceGeneration, Endpoint: v.Endpoint, Eligible: v.Eligible, Ready: v.Ready, Running: v.Running, AgentType: v.AgentType}, v.Terminated, nil
}
func (c DockerIdentity) Resolve(ctx context.Context, l d.ResourceLocator, proof string) (d.ResourceFacts, error) {
	if proof == "" {
		return d.ResourceFacts{}, ErrNativeIdentity
	}
	facts, _, err := c.request(ctx, l, proof, "", false)
	return facts, err
}
func (c DockerIdentity) Observe(ctx context.Context, v d.Instance) (d.ResourceFacts, error) {
	facts, _, err := c.request(ctx, d.ResourceLocator{EnvironmentID: v.EnvironmentID, WorkspaceID: v.WorkspaceID, AgentConfigID: v.AgentConfigID, ReplicaID: v.ReplicaID, ResourceUID: v.ResourceUID}, "", v.ResourceGeneration, true)
	return facts, err
}
func (c DockerIdentity) Terminated(ctx context.Context, v d.Instance) (bool, error) {
	_, terminated, err := c.request(ctx, d.ResourceLocator{EnvironmentID: v.EnvironmentID, WorkspaceID: v.WorkspaceID, AgentConfigID: v.AgentConfigID, ReplicaID: v.ReplicaID, ResourceUID: v.ResourceUID}, "", v.ResourceGeneration, true)
	return terminated, err
}
