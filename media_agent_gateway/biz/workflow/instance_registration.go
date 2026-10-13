package workflow

import (
	"context"
	"errors"
	"media_agent/hertz_infra/sessioncredential"
	d "media_agent/media_agent_gateway/biz/domain/agent_instance"
	svc "media_agent/media_agent_gateway/biz/service/agent_instance"
	"strings"
)

type EnvironmentAuthority interface {
	Environment(context.Context, string, string) (string, error)
}
type InstanceRegistration struct {
	credentials AgentCredentialVerifier
	authority   AgentTokenAuthority
	placement   EnvironmentAuthority
	native      d.ResourceAuthority
	instances   *svc.Service
	keys        *sessioncredential.Ring
}
type RegistrationCommand struct {
	Locator                   d.ResourceLocator
	IncarnationID, Generation string
	ExpectedVersion           int64
}
type RegistrationIdentity struct {
	Facts   d.ResourceFacts
	Version int64
}
type RegistrationResult struct {
	Instance   *d.Instance
	Credential string
}

func NewInstanceRegistration(credentials AgentCredentialVerifier, authority AgentTokenAuthority, placement EnvironmentAuthority, native d.ResourceAuthority, instances *svc.Service, keys *sessioncredential.Ring) *InstanceRegistration {
	return &InstanceRegistration{credentials, authority, placement, native, instances, keys}
}
func (w *InstanceRegistration) stable(ctx context.Context, token, ws, cfg string) (string, error) {
	if w == nil || w.credentials == nil || w.authority == nil || w.placement == nil || w.native == nil || w.instances == nil || w.keys == nil {
		return "", ErrMetadataUnavailable
	}
	principal, err := w.credentials.VerifyAgentCredential(ctx, token)
	if err != nil {
		return "", err
	}
	id, ok := strings.CutPrefix(principal.ActorID, "Agent:")
	if !ok || id != cfg || principal.UserID == "" || principal.OrganizationID == "" || principal.CredentialID == "" {
		return "", ErrMetadataForbidden
	}
	facts, err := w.authority.ResolveAgentIdentity(ctx, id)
	if err != nil {
		return "", ErrMetadataUnavailable
	}
	if facts.ConfigurationID != id || facts.WorkspaceID != ws || facts.OwnerUserID != principal.UserID || facts.OrganizationID != principal.OrganizationID || facts.ActorID != principal.ActorID || !facts.OwnerAuthorized || facts.State != "running" {
		return "", ErrMetadataForbidden
	}
	return principal.UserID, nil
}
func (w *InstanceRegistration) Resolve(ctx context.Context, token, proof string, l d.ResourceLocator) (RegistrationIdentity, error) {
	user, err := w.stable(ctx, token, l.WorkspaceID, l.AgentConfigID)
	if err != nil {
		return RegistrationIdentity{}, err
	}
	environment, err := w.placement.Environment(ctx, user, l.WorkspaceID)
	if err != nil {
		return RegistrationIdentity{}, err
	}
	if environment != l.EnvironmentID {
		return RegistrationIdentity{}, ErrMetadataForbidden
	}
	facts, err := w.native.Resolve(ctx, l, proof)
	if err != nil {
		return RegistrationIdentity{}, err
	}
	if facts.Locator.EnvironmentID != environment || facts.Locator.WorkspaceID != l.WorkspaceID || facts.Locator.AgentConfigID != l.AgentConfigID || facts.Locator.ReplicaID != l.ReplicaID || !facts.Eligible || !facts.Running || facts.Generation == "" {
		return RegistrationIdentity{}, ErrMetadataForbidden
	}
	current, err := w.instances.Get(ctx, environment, l.WorkspaceID, l.AgentConfigID, l.ReplicaID)
	if errors.Is(err, d.ErrNotFound) {
		return RegistrationIdentity{Facts: facts}, nil
	}
	if err != nil {
		return RegistrationIdentity{}, err
	}
	return RegistrationIdentity{facts, current.Version}, nil
}
func (w *InstanceRegistration) Register(ctx context.Context, token, proof string, cmd RegistrationCommand) (RegistrationResult, error) {
	if cmd.IncarnationID == "" || len(cmd.IncarnationID) > 128 || cmd.Generation == "" || len(cmd.Generation) > 256 {
		return RegistrationResult{}, d.ErrInvalid
	}
	identity, err := w.Resolve(ctx, token, proof, cmd.Locator)
	if err != nil {
		return RegistrationResult{}, err
	}
	if identity.Facts.Generation != cmd.Generation || identity.Facts.Locator.ResourceUID != cmd.Locator.ResourceUID {
		return RegistrationResult{}, d.ErrConflict
	}
	f := identity.Facts
	l := f.Locator
	candidate := d.Instance{EnvironmentID: l.EnvironmentID, WorkspaceID: l.WorkspaceID, AgentConfigID: l.AgentConfigID, AgentType: f.AgentType, ReplicaID: l.ReplicaID, IncarnationID: cmd.IncarnationID, ResourceUID: l.ResourceUID, ResourceGeneration: f.Generation, ResourceName: l.ResourceName, Namespace: l.Namespace, Endpoint: f.Endpoint, CredentialKeyID: w.keys.Active, Admission: "active"}
	credential, err := w.credential(candidate)
	if err != nil {
		return RegistrationResult{}, err
	}
	candidate.CredentialHash = sessioncredential.Hash(credential)
	instance, err := w.instances.Register(ctx, candidate, cmd.ExpectedVersion)
	if err != nil {
		return RegistrationResult{}, err
	}
	credential, err = w.credential(*instance)
	if err != nil || !sessioncredential.Matches(credential, instance.CredentialHash) {
		return RegistrationResult{}, ErrMetadataUnavailable
	}
	return RegistrationResult{instance, credential}, nil
}
func (w *InstanceRegistration) credential(v d.Instance) (string, error) {
	return w.keys.Derive("it1", v.CredentialKeyID, v.EnvironmentID, v.WorkspaceID, v.AgentConfigID, v.ReplicaID, v.IncarnationID, v.ResourceUID, v.ResourceGeneration)
}

// VerifyCurrent also checks placement and native generation; credentials cannot outlive their resource.
func (w *InstanceRegistration) VerifyCurrent(ctx context.Context, token, credential, ws, cfg, replica, inc string) (*d.Instance, string, error) {
	user, err := w.stable(ctx, token, ws, cfg)
	if err != nil {
		return nil, "", err
	}
	env, err := w.placement.Environment(ctx, user, ws)
	if err != nil {
		return nil, "", err
	}
	v, err := w.instances.Get(ctx, env, ws, cfg, replica)
	if err != nil {
		return nil, "", err
	}
	if v.IncarnationID != inc || !sessioncredential.Matches(credential, v.CredentialHash) {
		return nil, "", ErrMetadataForbidden
	}
	facts, err := w.native.Observe(ctx, *v)
	if err != nil {
		return nil, "", err
	}
	if facts.Locator.ResourceUID != v.ResourceUID || facts.Generation != v.ResourceGeneration || !facts.Running {
		return nil, "", ErrMetadataForbidden
	}
	return v, user, nil
}
