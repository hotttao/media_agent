package application_agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"media_agent/hertz_infra/authn/appcredential"
	config "media_agent/media_control/biz/domain/agent_configuration"
	d "media_agent/media_control/biz/domain/workspace"
	ws "media_agent/media_control/biz/service/workspace"
	"regexp"
	"strings"
)

var ErrInvalid = errors.New("invalid application agent configuration")

type Tx interface {
	RunInTx(context.Context, func(context.Context) error) error
}
type Workspaces interface {
	PrepareCreate(context.Context, ws.CreateCommand) (*d.ConfiguredWorkspace, error)
	StorePrepared(context.Context, *d.ConfiguredWorkspace) error
	Get(context.Context, string, string) (*d.ConfiguredWorkspace, error)
}
type Bindings interface {
	Lock(context.Context, d.ApplicationBinding) error
	Get(context.Context, d.ApplicationBinding) (d.ApplicationBinding, error)
	ByWorkspace(context.Context, d.ApplicationBinding) (d.ApplicationBinding, error)
	Create(context.Context, d.ApplicationBinding) error
}
type Metadata interface {
	ConfirmInTransaction(context.Context, config.Snapshot, string, string, string, int64) (*config.Release, error)
}
type Flow struct {
	Workspaces Workspaces
	Bindings   Bindings
	Metadata   Metadata
	Tx         Tx
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func key(p appcredential.Principal, name string) d.ApplicationBinding {
	return d.ApplicationBinding{ApplicationID: p.ApplicationID, UserID: p.UserID, TeamID: p.TeamID, Name: name}
}
func (f *Flow) Ensure(ctx context.Context, p appcredential.Principal, name string, cmd ws.CreateCommand) (*d.ConfiguredWorkspace, bool, error) {
	if !namePattern.MatchString(name) || p.UserID == "" || p.ApplicationID == "" || p.TeamID == "" || p.OrganizationID == "" {
		return nil, false, ErrInvalid
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return nil, false, ErrInvalid
	}
	agent, e := d.ParseAgentType(cmd.AgentType)
	if e != nil {
		return nil, false, ErrInvalid
	}
	if _, _, _, e = d.NormalizeModelSettings(string(agent), cmd.ModelProtocol, cmd.ModelName, cmd.ModelBaseURL); e != nil {
		return nil, false, ErrInvalid
	}
	if cmd.ModelConnectionID != "" && (strings.TrimSpace(cmd.APIKey) != "" || string(agent) != "codex") {
		return nil, false, ErrInvalid
	}
	cmd.UserID = p.UserID
	cmd.TeamID = p.TeamID
	cmd.OrganizationID = p.OrganizationID
	raw, _ := json.Marshal(cmd)
	sum := sha256.Sum256(raw)
	b := key(p, name)
	b.Digest = hex.EncodeToString(sum[:])
	var out *d.ConfiguredWorkspace
	created := false
	// Resolve current authorization before entering the local transaction.
	prepared, e := f.Workspaces.PrepareCreate(ctx, cmd)
	if e != nil {
		return nil, false, e
	}
	e = f.Tx.RunInTx(ctx, func(ctx context.Context) error {
		if e := f.Bindings.Lock(ctx, b); e != nil {
			return e
		}
		old, e := f.Bindings.Get(ctx, b)
		if e == nil {
			if old.Digest != b.Digest {
				return d.ErrConflict
			}
			b = old
			return nil
		}
		if !errors.Is(e, d.ErrNotFound) {
			return e
		}
		out = prepared
		if e = f.Workspaces.StorePrepared(ctx, out); e != nil {
			return e
		}
		if f.Metadata != nil {
			snapshot := config.Snapshot{WorkspaceID: out.Workspace.ID, AgentConfigID: out.AgentConfiguration.ID, AgentType: out.AgentType, PlatformPromptTemplateVersion: "2", Prompt: &config.PromptSelection{Mode: "none"}, Resources: []config.Resource{}}
			release, e := f.Metadata.ConfirmInTransaction(ctx, snapshot, p.UserID, "initial-workspace", "initial-empty-selection-v1", 1)
			if e != nil {
				return e
			}
			out.AgentConfiguration.Version = release.SelectionRevision
		}
		b.WorkspaceID = out.Workspace.ID
		if e = f.Bindings.Create(ctx, b); e != nil {
			return e
		}
		created = true
		return nil
	})
	if e != nil {
		return nil, false, e
	}
	if !created {
		out, e = f.checked(ctx, p, b)
	}
	return out, created, e
}
func (f *Flow) checked(ctx context.Context, p appcredential.Principal, b d.ApplicationBinding) (*d.ConfiguredWorkspace, error) {
	v, e := f.Workspaces.Get(ctx, b.WorkspaceID, p.UserID)
	if e != nil {
		return nil, e
	}
	if v.OwnerUserID != p.UserID || v.TeamID != p.TeamID || v.OrganizationID != p.OrganizationID {
		return nil, d.ErrNotFound
	}
	return v, nil
}
func (f *Flow) Get(ctx context.Context, p appcredential.Principal, name string) (*d.ConfiguredWorkspace, error) {
	b, e := f.Bindings.Get(ctx, key(p, name))
	if e != nil {
		return nil, e
	}
	return f.checked(ctx, p, b)
}
func (f *Flow) ByWorkspace(ctx context.Context, p appcredential.Principal, id string) (*d.ConfiguredWorkspace, error) {
	b := key(p, "")
	b.WorkspaceID = id
	b, e := f.Bindings.ByWorkspace(ctx, b)
	if e != nil {
		return nil, e
	}
	return f.checked(ctx, p, b)
}
