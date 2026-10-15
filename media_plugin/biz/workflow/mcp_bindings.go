package workflow

import (
	"context"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
	service "media_agent/media_plugin/biz/service/plugin_binding"
	"net/url"
	"strings"
)

type Workspace = d.Workspace
type Definition = d.Definition
type Resource = d.Resource
type Release = d.Release
type Authority interface {
	Workspace(context.Context, string, string, string) (Workspace, error)
	Definition(context.Context, string, int64) (Definition, error)
	Release(context.Context, string, string, string, string) (Release, error)
	OAuthToken(context.Context, string, string) (string, error)
}
type Secrets interface {
	Encrypt(string) ([]byte, error)
	Decrypt([]byte) (string, error)
}
type Bindings struct {
	Service   *service.Service
	Authority Authority
	Secrets   Secrets
}
type Command struct {
	Binding         d.Binding
	UserID          string
	ExpectedVersion int64
	Credential      *string
}

func (w *Bindings) definition(ctx context.Context, workspace Workspace, binding *d.Binding, selecting bool) (Definition, error) {
	current, err := w.Authority.Definition(ctx, binding.DefinitionID, 0)
	if err != nil {
		return Definition{}, err
	}
	if current.ID != binding.DefinitionID || current.OrganizationID != "" && current.OrganizationID != workspace.OrganizationID {
		return Definition{}, d.ErrForbidden
	}
	if selecting && current.Status != "enabled" {
		return Definition{}, d.ErrInvalid
	}
	fixed, err := w.Authority.Definition(ctx, binding.DefinitionID, binding.DefinitionVersion)
	if err != nil {
		return Definition{}, err
	}
	if fixed.ID != current.ID || fixed.Version != binding.DefinitionVersion || fixed.OrganizationID != current.OrganizationID || fixed.Transport == "platform" || selecting && fixed.Status != "enabled" {
		return Definition{}, d.ErrInvalid
	}
	return fixed, nil
}
func (w *Bindings) Put(ctx context.Context, cmd Command) (*d.Binding, error) {
	v := cmd.Binding
	ws, err := w.Authority.Workspace(ctx, cmd.UserID, v.WorkspaceID, "manage")
	if err != nil {
		return nil, err
	}
	if v.ID != "" {
		previous, e := w.Service.Get(ctx, v.ID)
		if e != nil {
			return nil, e
		}
		if previous.WorkspaceID != ws.ID || previous.DefinitionID != v.DefinitionID {
			return nil, d.ErrForbidden
		}
	}
	definition, err := w.definition(ctx, ws, &v, v.Enabled)
	if err != nil {
		return nil, err
	}
	if cmd.Credential != nil && (len(*cmd.Credential) > 16<<10 || strings.ContainsAny(*cmd.Credential, "\r\n\x00")) {
		return nil, d.ErrInvalid
	}
	switch definition.AuthenticationType {
	case "bearer":
		v.CredentialHeader = "Authorization"
	case "api_key":
		if !service.ValidCredentialHeader(v.CredentialHeader) {
			return nil, d.ErrInvalid
		}
		v.CredentialHeader = service.CanonicalHeader(v.CredentialHeader)
	case "oauth", "none":
		if cmd.Credential != nil && *cmd.Credential != "" || v.CredentialHeader != "" {
			return nil, d.ErrInvalid
		}
		cleared := ""
		cmd.Credential = &cleared
	default:
		return nil, d.ErrInvalid
	}
	var encrypted *[]byte
	if cmd.Credential != nil {
		value, e := w.Secrets.Encrypt(*cmd.Credential)
		if e != nil {
			return nil, e
		}
		encrypted = &value
	}
	v.UpdatedBy = cmd.UserID
	return w.Service.Put(ctx, v, cmd.ExpectedVersion, encrypted)
}
func (w *Bindings) Get(ctx context.Context, user, workspace, id string, version int64, selecting bool) (*d.Binding, error) {
	permission := "view"
	if selecting {
		permission = "manage"
	}
	ws, err := w.Authority.Workspace(ctx, user, workspace, permission)
	if err != nil {
		return nil, err
	}
	current, err := w.Service.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.WorkspaceID != ws.ID {
		return nil, d.ErrForbidden
	}
	value := current
	if version > 0 {
		value, err = w.Service.Version(ctx, id, version)
		if err != nil {
			return nil, err
		}
	}
	if value.WorkspaceID != ws.ID || selecting && (!current.Enabled || !value.Enabled) {
		return nil, d.ErrInvalid
	}
	definition, err := w.definition(ctx, ws, value, selecting)
	if err != nil {
		return nil, err
	}
	if selecting && (definition.AuthenticationType == "api_key" || definition.AuthenticationType == "bearer") && value.CredentialID == "" {
		return nil, d.ErrInvalid
	}
	return value, nil
}
func (w *Bindings) List(ctx context.Context, user, workspace string) ([]d.Binding, error) {
	ws, err := w.Authority.Workspace(ctx, user, workspace, "view")
	if err != nil {
		return nil, err
	}
	return w.Service.List(ctx, ws.ID)
}
func (w *Bindings) Versions(ctx context.Context, user, workspace, id string) ([]d.Binding, error) {
	if _, err := w.Get(ctx, user, workspace, id, 0, false); err != nil {
		return nil, err
	}
	return w.Service.Versions(ctx, id)
}
func (w *Bindings) Connection(ctx context.Context, user string, v *d.Binding) (string, string, error) {
	definition, err := w.Authority.Definition(ctx, v.DefinitionID, v.DefinitionVersion)
	if err != nil {
		return "", "", err
	}
	switch definition.AuthenticationType {
	case "oauth":
		u, err := url.Parse(definition.Endpoint)
		if err != nil {
			return "", "", d.ErrInvalid
		}
		origin := u.Scheme + "://" + u.Host
		token, err := w.Authority.OAuthToken(ctx, user, origin)
		if err != nil && err != d.ErrReauth {
			return "", "", err
		}
		state := "connected"
		if token == "" || err == d.ErrReauth {
			state = "needs_reauth"
		}
		return state, "/api/mcp-oauth/authorize?" + url.Values{"server_origin": {origin}, "workspace_id": {v.WorkspaceID}}.Encode(), nil
	case "bearer", "api_key":
		if v.CredentialID == "" {
			return "not_configured", "", nil
		}
	}
	return "connected", "", nil
}
