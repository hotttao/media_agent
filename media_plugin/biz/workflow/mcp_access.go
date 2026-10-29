package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	d "media_agent/media_plugin/biz/domain/plugin_binding"
	"net/url"
	"strconv"
)

type Scope struct {
	UserID, WorkspaceID, AgentConfigID, Target, SessionID string
	ContextVersion                                        int64
}
type Grant struct {
	Binding    d.Binding
	Definition Definition
	Headers    map[string]string
	OAuthProxy bool
}
type Access struct {
	SnapshotVersion, AuthorizationRevision string
	Grants                                 []Grant
	RestrictedDefinitionIDs                []string
}

func (w *Bindings) Resolve(ctx context.Context, scope Scope) (Access, error) {
	if scope.UserID == "" || scope.WorkspaceID == "" || scope.AgentConfigID == "" || scope.SessionID == "" || scope.ContextVersion < 1 {
		return Access{}, d.ErrInvalid
	}
	ws, err := w.Authority.Workspace(ctx, scope.UserID, scope.WorkspaceID, "view")
	if err != nil {
		return Access{}, err
	}
	if ws.AgentConfigID != scope.AgentConfigID {
		return Access{}, d.ErrForbidden
	}
	result := Access{SnapshotVersion: "platform-only", AuthorizationRevision: "platform-only", Grants: []Grant{}}
	// Legacy sessions without a metadata release cannot acquire any external source.
	if scope.Target == "" {
		return result, nil
	}
	release, err := w.Authority.Release(ctx, scope.UserID, ws.ID, scope.AgentConfigID, scope.Target)
	if err != nil {
		return Access{}, err
	}
	if release.WorkspaceID != ws.ID || release.AgentConfigID != scope.AgentConfigID || release.Target != scope.Target || release.ContextVersion != scope.ContextVersion {
		return Access{}, d.ErrForbidden
	}
	result.SnapshotVersion = release.Target
	vector := ""
	seen := map[string]bool{}
	for _, ref := range release.Resources {
		if ref.Kind != "media_plugin.mcp_binding" {
			continue
		}
		if seen[ref.ID] || len(seen) >= 128 {
			return Access{}, d.ErrInvalid
		}
		seen[ref.ID] = true
		current, err := w.Service.Get(ctx, ref.ID)
		if err != nil {
			return Access{}, err
		}
		if current.WorkspaceID != ws.ID {
			return Access{}, d.ErrForbidden
		}
		result.RestrictedDefinitionIDs = append(result.RestrictedDefinitionIDs, current.DefinitionID)
		vector += current.ID + ":" + strconv.FormatInt(current.Version, 10) + ";"
		if !current.Enabled {
			continue
		}
		fixed, err := w.Service.Version(ctx, ref.ID, ref.Version)
		if err != nil {
			return Access{}, err
		}
		if fixed.WorkspaceID != ws.ID || fixed.Checksum != ref.Checksum || !fixed.Enabled {
			return Access{}, d.ErrForbidden
		}
		definition, err := w.definition(ctx, ws, fixed, true)
		if err == d.ErrInvalid {
			continue
		}
		if err != nil {
			return Access{}, err
		}
		grant := Grant{Binding: *fixed, Definition: definition, Headers: map[string]string{}}
		switch definition.AuthenticationType {
		case "bearer", "api_key":
			if fixed.CredentialID == "" {
				continue
			}
			encrypted, err := w.Service.Credential(ctx, fixed.CredentialID, fixed.ID)
			if err != nil {
				return Access{}, err
			}
			secret, err := w.Secrets.Decrypt(encrypted)
			if err != nil {
				return Access{}, d.ErrUnavailable
			}
			if secret == "" {
				continue
			}
			if definition.AuthenticationType == "bearer" {
				grant.Headers["Authorization"] = "Bearer " + secret
			} else {
				grant.Headers[fixed.CredentialHeader] = secret
			}
		case "oauth":
			grant.OAuthProxy = true
		case "none":
		default:
			return Access{}, d.ErrInvalid
		}
		result.Grants = append(result.Grants, grant)
	}
	hash := sha256.Sum256([]byte(vector))
	result.AuthorizationRevision = hex.EncodeToString(hash[:])
	return result, nil
}
func (w *Bindings) OAuthBackend(ctx context.Context, scope Scope, binding string, version int64) (Definition, map[string]string, error) {
	access, err := w.Resolve(ctx, scope)
	if err != nil {
		return Definition{}, nil, err
	}
	for _, grant := range access.Grants {
		if grant.Binding.ID != binding || grant.Binding.Version != version || !grant.OAuthProxy {
			continue
		}
		parsed, err := url.Parse(grant.Definition.Endpoint)
		if err != nil {
			return Definition{}, nil, d.ErrInvalid
		}
		token, err := w.Authority.OAuthToken(ctx, scope.UserID, parsed.Scheme+"://"+parsed.Host)
		if err != nil {
			return Definition{}, nil, err
		}
		if token == "" {
			return Definition{}, nil, d.ErrReauth
		}
		return grant.Definition, map[string]string{"Authorization": "Bearer " + token}, nil
	}
	return Definition{}, nil, d.ErrForbidden
}
