package platform_administration

import (
	"context"
	"errors"
	"testing"

	domain "media_agent/media_control/biz/domain/platform_administration"
)

type fakeAdminWorkspaceRepository struct{ received domain.AdminWorkspaceFilter }

func (r *fakeAdminWorkspaceRepository) ListAdminWorkspaces(_ context.Context, filter domain.AdminWorkspaceFilter) ([]domain.AdminWorkspace, int64, error) {
	r.received = filter
	return []domain.AdminWorkspace{{WorkspaceID: "workspace-1", OwnerUserID: filter.OwnerUserID, AgentType: filter.AgentType}}, 1, nil
}

func TestAdminWorkspaceQueryAppliesFiltersAndPagination(t *testing.T) {
	repository := &fakeAdminWorkspaceRepository{}
	service := NewAdminWorkspaceService(repository)
	values, total, filter, err := service.List(context.Background(), domain.Actor{UserID: "admin", Role: "admin"}, "owner-1", "running", "codex", 25, 10)
	if err != nil || total != 1 || len(values) != 1 || filter.PageSize != 25 || repository.received.PageOffset != 10 {
		t.Fatalf("values=%+v total=%d filter=%+v received=%+v err=%v", values, total, filter, repository.received, err)
	}
}
func TestAdminWorkspaceQueryRejectsOrdinaryUser(t *testing.T) {
	service := NewAdminWorkspaceService(&fakeAdminWorkspaceRepository{})
	if _, _, _, err := service.List(context.Background(), domain.Actor{UserID: "user", Role: "user"}, "", "", "", 0, 0); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ordinary user accepted: %v", err)
	}
}
