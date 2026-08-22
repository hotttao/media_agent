package teamwork

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_control/biz/domain/teamwork"
	workspace "media_agent/media_control/biz/domain/workspace"
)

type fakeTopologyRepository struct {
	value *domain.Topology
}

func (r *fakeTopologyRepository) Create(_ context.Context, value *domain.Topology) error {
	copy := *value
	r.value = &copy
	return nil
}
func (r *fakeTopologyRepository) ListOwned(context.Context, string, string) ([]domain.Topology, error) {
	if r.value == nil {
		return nil, nil
	}
	return []domain.Topology{*r.value}, nil
}
func (r *fakeTopologyRepository) GetOwned(context.Context, string, string) (*domain.Topology, error) {
	if r.value == nil {
		return nil, domain.ErrNotFound
	}
	copy := *r.value
	return &copy, nil
}
func (r *fakeTopologyRepository) Update(_ context.Context, value *domain.Topology, _ int64) error {
	copy := *value
	r.value = &copy
	return nil
}
func (r *fakeTopologyRepository) Delete(_ context.Context, value *domain.Topology, _ int64) error {
	copy := *value
	r.value = &copy
	return nil
}

type fakeWorkspaceRepository struct {
	value *workspace.ConfiguredWorkspace
}

func (r *fakeWorkspaceRepository) Create(context.Context, *workspace.Workspace, *workspace.AgentConfiguration) error {
	return nil
}
func (r *fakeWorkspaceRepository) ListOwned(context.Context, string) ([]workspace.ConfiguredWorkspace, error) {
	return nil, nil
}
func (r *fakeWorkspaceRepository) GetOwned(_ context.Context, id, owner string) (*workspace.ConfiguredWorkspace, error) {
	if r.value == nil || r.value.Workspace.ID != id || r.value.Workspace.OwnerUserID != owner {
		return nil, workspace.ErrNotFound
	}
	copy := *r.value
	return &copy, nil
}
func (r *fakeWorkspaceRepository) Update(context.Context, *workspace.Workspace, *workspace.AgentConfiguration) error {
	return nil
}

type directTx struct{}

func (directTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func configuredWorkspace() *workspace.ConfiguredWorkspace {
	return &workspace.ConfiguredWorkspace{
		Workspace:          workspace.Workspace{ID: "workspace-1", OwnerUserID: "owner-1", DesiredState: "running", Version: 1},
		AgentConfiguration: workspace.AgentConfiguration{ID: "config-1", WorkspaceID: "workspace-1", AgentID: "codex", AgentType: "codex", Version: 3},
	}
}

func activeTopology() *domain.Topology {
	return &domain.Topology{
		ID: "topology-1", WorkspaceID: "workspace-1", EntryMemberID: "writer", Status: domain.StatusActive, Version: 4,
		Members:   []domain.Member{{MemberID: "writer", AgentConfigID: "config-1", Name: "Writer"}},
		CreatedAt: time.Unix(10, 0).UTC(), UpdatedAt: time.Unix(20, 0).UTC(),
	}
}

func TestRuntimeContext(t *testing.T) {
	repository := &fakeTopologyRepository{value: activeTopology()}
	service := New(repository, directTx{}, &fakeWorkspaceRepository{value: configuredWorkspace()})

	value, err := service.RuntimeContext(context.Background(), "topology-1", "owner-1")
	if err != nil || value.ID != "topology-1" || value.Version != 4 || value.EntryMemberID != "writer" {
		t.Fatalf("value=%+v err=%v", value, err)
	}

	repository.value.Status = domain.StatusDeleted
	if _, err = service.RuntimeContext(context.Background(), "topology-1", "owner-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted topology runtime context: %v", err)
	}
}

func TestRuntimeContextRejectsForeignConfiguration(t *testing.T) {
	repository := &fakeTopologyRepository{}
	service := New(repository, directTx{}, &fakeWorkspaceRepository{value: configuredWorkspace()})
	members := `[{"member_id":"writer","agent_config_id":"foreign-config","name":"Writer"}]`
	_, err := service.Create(context.Background(), CreateCommand{Owner: "owner-1", WorkspaceID: "workspace-1", Name: "writers", EntryMemberID: "writer", MembersJSON: members, EdgesJSON: `[]`})
	if err == nil {
		t.Fatal("foreign agent configuration was accepted on create")
	}
	repository.value = activeTopology()
	_, err = service.Update(context.Background(), UpdateCommand{ID: "topology-1", Owner: "owner-1", Name: "writers", EntryMemberID: "writer", MembersJSON: members, EdgesJSON: `[]`, ExpectedVersion: 4})
	if err == nil {
		t.Fatal("foreign agent configuration was accepted on update")
	}
}
