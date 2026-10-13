package workflow

import (
	"context"
	"errors"
	"media_agent/media_agent_gateway/biz/shared/client"
	"strings"
)

var ErrMetadataForbidden = errors.New("metadata_permission_denied")
var ErrMetadataUnavailable = errors.New("metadata_unavailable")

type MetadataInstances interface {
	VerifyMetadataInstance(context.Context, string, string, string, string, string) error
	ListMetadataInstances(context.Context, string) ([]client.MetadataInstance, error)
}
type MetadataControl interface {
	Get(context.Context, string, client.MetadataQuery) (client.MetadataContext, error)
	Report(context.Context, string, client.MetadataResult) error
	Artifact(context.Context, string, client.MetadataQuery, string) (client.MetadataArtifact, error)
}
type AgentMetadata struct {
	credentials AgentCredentialVerifier
	authority   AgentTokenAuthority
	instances   MetadataInstances
	control     MetadataControl
	membership  interface {
		Membership(context.Context, string) ([]client.MetadataInstance, bool, error)
	}
	nativeGuard interface {
		AuthorizeMetadataInstance(context.Context, string, client.MetadataQuery) error
	}
}

func NewAgentMetadata(credentials AgentCredentialVerifier, authority AgentTokenAuthority, instances MetadataInstances, control MetadataControl) *AgentMetadata {
	return &AgentMetadata{credentials: credentials, authority: authority, instances: instances, control: control}
}
func (f *AgentMetadata) authorize(ctx context.Context, token, instanceCredential string, q client.MetadataQuery) (string, error) {
	if f == nil || f.credentials == nil || f.authority == nil || f.instances == nil || f.control == nil {
		return "", ErrMetadataUnavailable
	}
	p, err := f.credentials.VerifyAgentCredential(ctx, token)
	if err != nil {
		return "", err
	}
	id, ok := strings.CutPrefix(p.ActorID, "Agent:")
	if !ok || id == "" || p.UserID == "" || p.OrganizationID == "" || p.CredentialID == "" {
		return "", ErrMetadataForbidden
	}
	facts, err := f.authority.ResolveAgentIdentity(ctx, id)
	if err != nil {
		return "", ErrMetadataUnavailable
	}
	if facts.ConfigurationID != id || facts.ActorID != p.ActorID || facts.OwnerUserID != p.UserID || facts.OrganizationID != p.OrganizationID || !facts.OwnerAuthorized || facts.WorkspaceID != q.WorkspaceID || id != q.AgentConfigID || facts.State != "running" && facts.State != "stopped" {
		return "", ErrMetadataForbidden
	}
	if q.ReplicaID == "" || q.IncarnationID == "" {
		return "", ErrMetadataForbidden
	}
	if err := f.instances.VerifyMetadataInstance(ctx, q.WorkspaceID, id, q.ReplicaID, q.IncarnationID, instanceCredential); err != nil {
		if errors.Is(err, client.ErrMetadataInstanceUnverified) {
			return "", ErrMetadataForbidden
		}
		return "", ErrMetadataUnavailable
	}
	if f.nativeGuard != nil {
		if err := f.nativeGuard.AuthorizeMetadataInstance(ctx, p.UserID, q); err != nil {
			return "", err
		}
	}
	return p.UserID, nil
}
func (f *AgentMetadata) Get(ctx context.Context, token, instanceCredential string, q client.MetadataQuery) (client.MetadataContext, error) {
	user, err := f.authorize(ctx, token, instanceCredential, q)
	if err != nil {
		return client.MetadataContext{}, err
	}
	v, err := f.control.Get(ctx, user, q)
	if err != nil {
		return v, err
	}
	if v.WorkspaceID != q.WorkspaceID || v.AgentConfigID != q.AgentConfigID {
		return v, ErrMetadataUnavailable
	}
	if v.MetadataSchemaVersion != 1 {
		return v, nil
	}
	v.PlatformPrompt, v.PlatformPromptTemplateVersion, err = RenderPlatformPrompt(v)
	if err != nil {
		return client.MetadataContext{}, err
	}
	return v, nil
}
func (f *AgentMetadata) Report(ctx context.Context, token, instanceCredential string, v client.MetadataResult) error {
	q := client.MetadataQuery{WorkspaceID: v.WorkspaceID, AgentConfigID: v.AgentConfigID, ReplicaID: v.ReplicaID, IncarnationID: v.IncarnationID}
	user, err := f.authorize(ctx, token, instanceCredential, q)
	if err != nil {
		return err
	}
	return f.control.Report(ctx, user, v)
}
func (f *AgentMetadata) Artifact(ctx context.Context, token, instanceCredential string, q client.MetadataQuery, hash string) (client.MetadataArtifact, error) {
	user, err := f.authorize(ctx, token, instanceCredential, q)
	if err != nil {
		return client.MetadataArtifact{}, err
	}
	return f.control.Artifact(ctx, user, q, hash)
}
func (f *AgentMetadata) Instances(ctx context.Context, ws string) ([]client.MetadataInstance, error) {
	if f == nil || f.instances == nil {
		return nil, ErrMetadataUnavailable
	}
	return f.instances.ListMetadataInstances(ctx, ws)
}

func (f *AgentMetadata) WithNativeGuard(guard interface {
	AuthorizeMetadataInstance(context.Context, string, client.MetadataQuery) error
}) *AgentMetadata {
	f.nativeGuard = guard
	return f
}

func (f *AgentMetadata) WithMembership(m interface {
	Membership(context.Context, string) ([]client.MetadataInstance, bool, error)
}) *AgentMetadata {
	f.membership = m
	return f
}
func (f *AgentMetadata) Membership(ctx context.Context, ws string) ([]client.MetadataInstance, bool, error) {
	if f.membership != nil {
		return f.membership.Membership(ctx, ws)
	}
	v, e := f.Instances(ctx, ws)
	return v, false, e
}
