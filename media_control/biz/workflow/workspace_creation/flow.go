package workspace_creation

import (
	"context"
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
