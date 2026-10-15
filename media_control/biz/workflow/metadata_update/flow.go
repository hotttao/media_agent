package metadata_update

import (
	"context"
	"encoding/json"
	"errors"
	d "media_agent/media_control/biz/domain/agent_configuration"
	prompt "media_agent/media_control/biz/domain/prompt"
	workspace "media_agent/media_control/biz/domain/workspace"
	service "media_agent/media_control/biz/service/agent_configuration"
	"sort"
)

type WorkspaceAccess interface {
	CheckAccess(context.Context, string, string, string) (bool, error)
	Get(context.Context, string, string) (*workspace.ConfiguredWorkspace, error)
}
type Artifacts interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}
type InstanceReader interface {
	ListMetadataInstances(context.Context, string, string) ([]d.Instance, bool, error)
}
type Flow struct {
	service    *service.Service
	workspaces WorkspaceAccess
	artifacts  Artifacts
	instances  InstanceReader
	prompts    PromptLibrary
}

type PromptLibrary interface {
	SelectedVersion(context.Context, string, string, string, int64) (*prompt.Version, error)
	ReadableVersion(context.Context, string, string, string, int64) (*prompt.Version, error)
}

func New(s *service.Service, ws WorkspaceAccess, artifacts Artifacts, instances InstanceReader) *Flow {
	return &Flow{service: s, workspaces: ws, artifacts: artifacts, instances: instances}
}
func (f *Flow) WithPrompts(library PromptLibrary) *Flow { f.prompts = library; return f }
func (f *Flow) authorize(ctx context.Context, ws, user, permission string) (*workspace.ConfiguredWorkspace, error) {
	ok, err := f.workspaces.CheckAccess(ctx, ws, user, permission)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, d.ErrForbidden
	}
	return f.workspaces.Get(ctx, ws, user)
}

type Confirm struct {
	WorkspaceID, AgentConfigID, UserID, IdempotencyKey string
	ExpectedVersion                                    int64
	Resources                                          []d.ResourceReference
	Prompt                                             *d.PromptSelection
}

func (f *Flow) Confirm(ctx context.Context, cmd Confirm) (*d.Release, error) {
	if len(cmd.Resources) > 128 || len([]byte(cmd.WorkspaceID))+len([]byte(cmd.AgentConfigID)) > 127 {
		return nil, d.ErrInvalid
	}
	configured, err := f.authorize(ctx, cmd.WorkspaceID, cmd.UserID, "manage")
	if err != nil {
		return nil, err
	}
	if configured.AgentConfiguration.ID != cmd.AgentConfigID {
		return nil, d.ErrForbidden
	}
	refs := append([]d.ResourceReference(nil), cmd.Resources...)
	selection, refs, err := normalizePrompt(cmd.Prompt, refs)
	if err != nil {
		return nil, err
	}
	if len(refs) > 128 {
		return nil, d.ErrInvalid
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind == refs[j].Kind {
			return refs[i].ID < refs[j].ID
		}
		return refs[i].Kind < refs[j].Kind
	})
	encoded, _ := json.Marshal(struct {
		ID      string
		Version int64
		Refs    []d.ResourceReference
		Prompt  *d.PromptSelection `json:",omitempty"`
	}{cmd.AgentConfigID, cmd.ExpectedVersion, refs, cmd.Prompt})
	hash := service.Hash(encoded)
	if previous, err := f.service.Existing(ctx, cmd.AgentConfigID, cmd.IdempotencyKey, hash); err == nil {
		return previous, nil
	} else if !errors.Is(err, d.ErrNotFound) {
		return nil, err
	}
	if configured.AgentConfiguration.Version != cmd.ExpectedVersion {
		return nil, d.ErrConflict
	}
	snapshot := d.Snapshot{PlatformPromptTemplateVersion: "2", WorkspaceID: cmd.WorkspaceID, AgentConfigID: cmd.AgentConfigID, AgentType: configured.AgentType, Resources: []d.Resource{}}
	snapshot.Prompt = &selection
	if selection.Mode == "custom" {
		snapshot.ConfiguredPrompt = selection.Content
	}
	promptSet := false
	for i, ref := range refs {
		if ref.Version <= 0 || ref.ID == "" || i > 0 && refs[i-1].Kind == ref.Kind && refs[i-1].ID == ref.ID {
			return nil, d.ErrInvalid
		}
		if ref.Kind == "media_control.configured_prompt" {
			if promptSet {
				return nil, d.ErrInvalid
			}
			if f.prompts == nil {
				return nil, prompt.ErrUnavailable
			}
			promptSet = true
			version, err := f.prompts.SelectedVersion(ctx, ref.ID, cmd.UserID, configured.OrganizationID, ref.Version)
			if err != nil {
				return nil, promptError(err)
			}
			checksum := service.Hash([]byte(version.Content))
			if ref.Checksum != "" && ref.Checksum != checksum {
				return nil, d.ErrConflict
			}
			ref.Checksum = checksum
			snapshot.ConfiguredPrompt = version.Content
			snapshot.Resources = append(snapshot.Resources, d.Resource{ResourceReference: ref, Name: version.Name})
			continue
		}
		version, err := f.service.Resource(ctx, ref)
		if err != nil {
			return nil, err
		}
		if version.OwnerUserID != cmd.UserID || version.WorkspaceID != "" && version.WorkspaceID != cmd.WorkspaceID {
			return nil, d.ErrForbidden
		}
		var payload struct {
			Name, Content, Status string
			ContentPath           string `json:"content_path"`
			Checksum              string
			StoreType             string `json:"store_type"`
			CredentialReferenceID string `json:"credential_reference_id"`
		}
		if err := json.Unmarshal(version.Payload, &payload); err != nil {
			return nil, err
		}
		if payload.Status == "archived" {
			return nil, d.ErrInvalid
		}
		resource := d.Resource{ResourceReference: ref, Name: payload.Name}
		switch ref.Kind {
		case "media_control.skill":
			if payload.ContentPath != "sha256/"+payload.Checksum {
				return nil, d.ErrInvalid
			}
			body, err := f.artifacts.Get(ctx, payload.Checksum)
			if err != nil {
				return nil, err
			}
			resource.Checksum = service.Hash(body)
			resource.ContentRef = "sha256/" + resource.Checksum
		case "media_control.memory_store":
			// The selected descriptor retains immutable configuration and credential references,
			// never arbitrary provider configuration or secret values.
			descriptor, _ := json.Marshal(map[string]any{"id": ref.ID, "name": payload.Name, "store_type": payload.StoreType, "status": payload.Status, "credential_reference_id": payload.CredentialReferenceID, "configuration_ref": map[string]any{"kind": ref.Kind, "id": ref.ID, "version": ref.Version}})
			resource.Content = string(descriptor)
			resource.Checksum = service.Hash(descriptor)
		default:
			return nil, d.ErrInvalid
		}
		if ref.Checksum != "" && ref.Checksum != resource.Checksum {
			return nil, d.ErrConflict
		}
		snapshot.Resources = append(snapshot.Resources, resource)
	}
	return f.service.Confirm(ctx, snapshot, cmd.UserID, cmd.IdempotencyKey, hash, cmd.ExpectedVersion)
}

func normalizePrompt(input *d.PromptSelection, refs []d.ResourceReference) (d.PromptSelection, []d.ResourceReference, error) {
	selection := d.PromptSelection{Mode: "none"}
	var selected *d.ResourceReference
	for i := range refs {
		if refs[i].Kind == "media_control.configured_prompt" {
			if selected != nil {
				return selection, nil, d.ErrInvalid
			}
			selected = &refs[i]
		}
	}
	if input == nil {
		if selected != nil {
			selection = d.PromptSelection{Mode: "library", ResourceID: selected.ID, ResourceVersion: selected.Version}
		}
		return selection, refs, nil
	}
	selection = *input
	switch selection.Mode {
	case "none":
		if selected != nil || selection.Content != "" || selection.ResourceID != "" || selection.ResourceVersion != 0 {
			return selection, nil, d.ErrInvalid
		}
	case "custom":
		if selected != nil || selection.ResourceID != "" || selection.ResourceVersion != 0 || len(selection.Content) > 1<<20 {
			return selection, nil, d.ErrInvalid
		}
	case "library":
		if selection.Content != "" || selection.ResourceID == "" || selection.ResourceVersion <= 0 {
			return selection, nil, d.ErrInvalid
		}
		if selected == nil {
			refs = append(refs, d.ResourceReference{Kind: "media_control.configured_prompt", ID: selection.ResourceID, Version: selection.ResourceVersion})
		} else if selected.ID != selection.ResourceID || selected.Version != selection.ResourceVersion {
			return selection, nil, d.ErrInvalid
		}
	default:
		return selection, nil, d.ErrInvalid
	}
	return selection, refs, nil
}
func promptError(err error) error {
	switch {
	case errors.Is(err, prompt.ErrNotFound), errors.Is(err, prompt.ErrForbidden):
		return d.ErrForbidden
	case errors.Is(err, prompt.ErrInvalid), errors.Is(err, prompt.ErrArchived):
		return d.ErrInvalid
	default:
		return err
	}
}
func (f *Flow) Selection(ctx context.Context, ws, user string) (*d.Release, int64, error) {
	v, err := f.authorize(ctx, ws, user, "view")
	if err != nil {
		return nil, 0, err
	}
	release, _, err := f.service.Get(ctx, ws, v.AgentConfiguration.ID, "", 0, false)
	if err == nil {
		for _, ref := range release.Resources {
			if ref.Kind == "media_control.configured_prompt" {
				if f.prompts == nil {
					return nil, 0, prompt.ErrUnavailable
				}
				if _, err := f.prompts.ReadableVersion(ctx, ref.ID, user, v.OrganizationID, ref.Version); err != nil {
					return nil, 0, promptError(err)
				}
			}
		}
	}
	return release, v.AgentConfiguration.Version, err
}
func (f *Flow) Get(ctx context.Context, ws, id, user, target string, revision int64, forSession bool) (*d.Release, bool, error) {
	v, err := f.authorize(ctx, ws, user, "view")
	if err != nil {
		return nil, false, err
	}
	if v.AgentConfiguration.ID != id {
		return nil, false, d.ErrForbidden
	}
	return f.service.Get(ctx, ws, id, target, revision, forSession)
}
func (f *Flow) Report(ctx context.Context, user string, result d.ApplicationResult) error {
	v, err := f.authorize(ctx, result.WorkspaceID, user, "view")
	if err != nil {
		return err
	}
	if v.AgentConfiguration.ID != result.AgentConfigID {
		return d.ErrForbidden
	}
	return f.service.Report(ctx, result)
}
func (f *Flow) Upload(ctx context.Context, ws, user, hash string, body []byte) error {
	if _, err := f.authorize(ctx, ws, user, "manage"); err != nil {
		return err
	}
	if len(body) == 0 || len(body) > 8<<20 || service.Hash(body) != hash {
		return d.ErrInvalid
	}
	return f.artifacts.Put(ctx, hash, body)
}
func (f *Flow) Artifact(ctx context.Context, ws, id, user, target, hash string) ([]byte, error) {
	release, _, err := f.Get(ctx, ws, id, user, target, 0, true)
	if err != nil {
		return nil, err
	}
	for _, resource := range release.Resources {
		if resource.Kind == "media_control.skill" && resource.Checksum == hash {
			return f.artifacts.Get(ctx, hash)
		}
	}
	return nil, d.ErrForbidden
}
func (f *Flow) Progress(ctx context.Context, ws, user string) (service.Progress, error) {
	v, err := f.authorize(ctx, ws, user, "view")
	if err != nil {
		return service.Progress{}, err
	}
	if f.instances == nil {
		return service.Progress{}, errors.New("instance facts unavailable")
	}
	instances, complete, err := f.instances.ListMetadataInstances(ctx, ws, user)
	if err != nil {
		return service.Progress{}, err
	}
	return f.service.Progress(ctx, ws, v.AgentConfiguration.ID, instances, complete)
}
