package workload_target

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	contract "media_agent/hertz_infra/agentworkload"
	d "media_agent/media_control/biz/domain/runtime_environment"
	workspace "media_agent/media_control/biz/domain/workspace"
	service "media_agent/media_control/biz/service/runtime_environment"
)

type WorkspaceAccess interface {
	CheckAccess(context.Context, string, string, string) (bool, error)
	Get(context.Context, string, string) (*workspace.ConfiguredWorkspace, error)
}
type Profile struct {
	Image, CPU, Memory string
	Port               int32
}
type Policy struct {
	SharedSecret, StorageClass, StorageSize, GatewayURL, Driver, Namespace, SessionExchange, MetadataExchange string
	Profiles                                                                                                  map[string]Profile
}
type Flow struct {
	Service    *service.Service
	Workspaces WorkspaceAccess
	Policy     Policy
}

var ErrForbidden = errors.New("workload permission denied")
var ErrInvalid = errors.New("workload target invalid")

func (f *Flow) authorize(ctx context.Context, ws, user, action string) (*workspace.ConfiguredWorkspace, error) {
	ok, e := f.Workspaces.CheckAccess(ctx, ws, user, action)
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, ErrForbidden
	}
	return f.Workspaces.Get(ctx, ws, user)
}
func (f *Flow) Get(ctx context.Context, ws, user string) (*d.WorkloadTarget, error) {
	if _, e := f.authorize(ctx, ws, user, "manage"); e != nil {
		return nil, e
	}
	return f.Service.Workload(ctx, ws)
}
func (f *Flow) Ensure(ctx context.Context, ws, user string) (*d.WorkloadTarget, error) {
	configured, e := f.authorize(ctx, ws, user, "manage")
	if e != nil {
		return nil, e
	}
	existing, e := f.Service.Workload(ctx, ws)
	if e == nil {
		if existing.AgentConfigID != configured.AgentConfiguration.ID {
			return nil, d.ErrConflict
		}
		if existing.WorkspaceVersion == configured.Workspace.Version {
			return existing, nil
		}
		existing.Replicas = existing.DesiredReplicas
		if configured.DesiredState != "running" {
			existing.Replicas = 0
		}
		existing.WorkspaceVersion = configured.Workspace.Version
		body, _ := json.Marshal(struct {
			State   string
			Version int64
		}{configured.DesiredState, configured.Workspace.Version})
		hash := sha256.Sum256(body)
		return f.Service.SetWorkload(ctx, *existing, existing.Version, fmt.Sprintf("lifecycle:%d", configured.Workspace.Version), hex.EncodeToString(hash[:]))
	}
	if !errors.Is(e, d.ErrNotFound) {
		return nil, e
	}
	placement, e := f.Service.Placement(ctx, ws, configured.OwnerUserID)
	if errors.Is(e, d.ErrPlacementNotFound) {
		placement, e = f.Service.Reconcile(ctx, ws, configured.OwnerUserID)
	}
	if e != nil {
		return nil, e
	}
	if placement.Status != d.PlacementPlaced {
		return nil, ErrInvalid
	}
	profile, ok := f.Policy.Profiles[configured.AgentType]
	if !ok {
		return nil, ErrInvalid
	}
	n := contract.Name(placement.EnvironmentID, ws)
	replicas := int32(1)
	if configured.DesiredState == "stopped" {
		replicas = 0
	}
	spec := contract.Spec{SessionExchange: f.Policy.SessionExchange, MetadataExchange: f.Policy.MetadataExchange, Driver: f.Policy.Driver, Namespace: f.Policy.Namespace, WorkspaceID: ws, AgentConfigID: configured.AgentConfiguration.ID, EnvironmentID: placement.EnvironmentID, AgentType: configured.AgentType, Image: profile.Image, Port: profile.Port, CPU: profile.CPU, Memory: profile.Memory, StorageClass: f.Policy.StorageClass, StorageSize: f.Policy.StorageSize, GatewayURL: f.Policy.GatewayURL, IdentitySecret: n + "-identity", SharedSecret: f.Policy.SharedSecret, ClaimName: n + "-workspace", Replicas: replicas}
	if spec.Validate() != nil {
		return nil, ErrInvalid
	}
	body, e := json.Marshal(spec)
	if e != nil {
		return nil, e
	}
	value := d.WorkloadTarget{WorkspaceID: ws, AgentConfigID: spec.AgentConfigID, EnvironmentID: spec.EnvironmentID, OwnerUserID: configured.OwnerUserID, Spec: body, Replicas: replicas, DesiredReplicas: 1, WorkspaceVersion: configured.Workspace.Version}
	hash := sha256.Sum256(body)
	result, e := f.Service.SetWorkload(ctx, value, 0, "initial", hex.EncodeToString(hash[:]))
	if errors.Is(e, d.ErrConflict) {
		return f.Service.Workload(ctx, ws)
	}
	return result, e
}
func (f *Flow) Update(ctx context.Context, ws, user, key string, replicas int32, expected int64) (*d.WorkloadTarget, error) {
	configured, e := f.authorize(ctx, ws, user, "manage")
	if e != nil {
		return nil, e
	}
	if replicas < 0 || replicas > 64 || expected < 1 || key == "" {
		return nil, ErrInvalid
	}
	current, e := f.Service.Workload(ctx, ws)
	if e != nil {
		return nil, e
	}
	if current.AgentConfigID != configured.AgentConfiguration.ID {
		return nil, d.ErrConflict
	}
	request, _ := json.Marshal(struct {
		Replicas int32
		Expected int64
	}{replicas, expected})
	hash := sha256.Sum256(request)
	spec, e := contract.Decode(json.RawMessage(current.Spec))
	if e != nil {
		return nil, ErrInvalid
	}
	if spec.Driver == "docker" {
		return nil, ErrInvalid
	}
	current.DesiredReplicas = replicas
	current.Replicas = replicas
	if configured.DesiredState != "running" {
		current.Replicas = 0
	}
	current.WorkspaceVersion = configured.Workspace.Version
	return f.Service.SetWorkload(ctx, *current, expected, key, hex.EncodeToString(hash[:]))
}
