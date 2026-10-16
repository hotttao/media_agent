package workspace_creation

import (
	"context"
	"errors"
	"fmt"
	config "media_agent/media_control/biz/domain/agent_configuration"
	domain "media_agent/media_control/biz/domain/workspace"
	metadata "media_agent/media_control/biz/service/agent_configuration"
	workspace "media_agent/media_control/biz/service/workspace"
)

type Workspaces interface {
	PrepareCreate(context.Context, workspace.CreateCommand) (*domain.ConfiguredWorkspace, error)
	StorePrepared(context.Context, *domain.ConfiguredWorkspace) error
}
type InitialConfiguration interface {
	ConfirmInTransaction(context.Context, config.Snapshot, string, string, string, int64) (*config.Release, error)
}
type Flow struct {
	Workspaces Workspaces
	Metadata   InitialConfiguration
	Tx         metadata.TxRunner
}

func (f *Flow) Create(ctx context.Context, cmd workspace.CreateCommand) (*domain.ConfiguredWorkspace, error) {
	value, err := f.Workspaces.PrepareCreate(ctx, cmd)
	if err != nil {
		return nil, err
	}
	err = f.Tx.RunInTx(ctx, func(tx context.Context) error {
		if err := f.Workspaces.StorePrepared(tx, value); err != nil {
			return err
		}
		initial := config.Snapshot{WorkspaceID: value.Workspace.ID, AgentConfigID: value.AgentConfiguration.ID, AgentType: value.AgentType, PlatformPromptTemplateVersion: "2", Prompt: &config.PromptSelection{Mode: "none"}, Resources: []config.Resource{}}
		release, err := f.Metadata.ConfirmInTransaction(tx, initial, cmd.UserID, "initial-workspace", "initial-empty-selection-v1", 1)
		if err != nil {
			return err
		}
		value.AgentConfiguration.Version = release.SelectionRevision
		return nil
	})
	if err != nil {
		return nil, err
	}
	return value, nil
}

// Update keeps the selected metadata content while confirming an explicit credential change.
func (f *Flow) Update(ctx context.Context, cmd workspace.UpdateCommand) (*domain.ConfiguredWorkspace, error) {
	w := f.Workspaces.(interface {
		PrepareUpdate(context.Context, workspace.UpdateCommand) (*domain.ConfiguredWorkspace, error)
		StoreUpdate(context.Context, *domain.ConfiguredWorkspace) error
	})
	v, e := w.PrepareUpdate(ctx, cmd)
	if e != nil {
		return nil, e
	}
	reader := f.Metadata.(interface {
		Get(context.Context, string, string, string, int64, bool) (*config.Release, bool, error)
	})
	previous, _, e := reader.Get(ctx, v.Workspace.ID, v.AgentConfiguration.ID, "", 0, false)
	if e != nil && !errors.Is(e, config.ErrNotFound) {
		return nil, e
	}
	confirm := v.ModelConnectionID != "" || cmd.ModelConnectionID != nil || cmd.ModelProtocol != nil || cmd.ModelName != nil || cmd.ModelBaseURL != nil
	e = f.Tx.RunInTx(ctx, func(tx context.Context) error {
		if e := w.StoreUpdate(tx, v); e != nil {
			return e
		}
		if !confirm || (previous != nil && previous.SelectionRevision == v.AgentConfiguration.Version) {
			return nil
		}
		snapshot := config.Snapshot{WorkspaceID: v.Workspace.ID, AgentConfigID: v.AgentConfiguration.ID, AgentType: v.AgentType, PlatformPromptTemplateVersion: "2", Prompt: &config.PromptSelection{Mode: "none"}, Resources: []config.Resource{}}
		if previous != nil {
			snapshot = previous.Snapshot
			snapshot.AgentType = v.AgentType
		}
		release, e := f.Metadata.ConfirmInTransaction(tx, snapshot, cmd.UserID, fmt.Sprintf("model-binding-%d", v.AgentConfiguration.Version), fmt.Sprintf("model-binding-%d", v.AgentConfiguration.Version), v.AgentConfiguration.Version)
		if e != nil {
			return e
		}
		v.AgentConfiguration.Version = release.SelectionRevision
		return nil
	})
	return v, e
}
