package teamwork_runtime

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_agent_gateway/biz/dal"
	"media_agent/media_agent_gateway/biz/dal/model"
	domain "media_agent/media_agent_gateway/biz/domain/teamwork_runtime"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) ApplyEvent(ctx context.Context, event domain.Event, now time.Time) (domain.ApplyResult, error) {
	db := dal.FromContext(ctx, r.db)
	inbox := &model.TeamworkTopologyInbox{EventID: event.ID, WorkspaceID: event.WorkspaceID, TopologyID: event.TopologyID, TopologyVersion: event.TopologyVersion, Status: event.Status, ProcessedAt: now}
	insert := db.Clauses(clause.OnConflict{DoNothing: true}).Create(inbox)
	if insert.Error != nil {
		return domain.ApplyResult{}, insert.Error
	}
	if insert.RowsAffected == 0 {
		return domain.ApplyResult{Duplicate: true}, nil
	}
	row := &model.TeamworkTopologyVersion{TopologyID: event.TopologyID, WorkspaceID: event.WorkspaceID, Version: event.TopologyVersion, Status: event.Status, UpdatedAt: now}
	upsert := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "topology_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"workspace_id", "version", "status", "updated_at"}),
		Where:     clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "teamwork_topology_versions.version < EXCLUDED.version"}}},
	}).Create(row)
	if upsert.Error != nil {
		return domain.ApplyResult{}, upsert.Error
	}
	if upsert.RowsAffected == 0 {
		var current model.TeamworkTopologyVersion
		if err := db.Where("topology_id = ?", event.TopologyID).First(&current).Error; err != nil {
			return domain.ApplyResult{}, err
		}
		if current.WorkspaceID != event.WorkspaceID {
			return domain.ApplyResult{}, domain.ErrWorkspaceMismatch
		}
	}
	return domain.ApplyResult{Advanced: upsert.RowsAffected > 0}, nil
}

func (r *Repository) Get(ctx context.Context, topologyID string) (*domain.Version, error) {
	var row model.TeamworkTopologyVersion
	err := dal.FromContext(ctx, r.db).Where("topology_id = ?", topologyID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &domain.Version{TopologyID: row.TopologyID, WorkspaceID: row.WorkspaceID, Version: row.Version, Status: row.Status, UpdatedAt: row.UpdatedAt}, nil
}

func (r *Repository) AuthorizeTopology(ctx context.Context, workspaceID, topologyID string, version int64) error {
	value, err := r.Get(ctx, topologyID)
	if err != nil || !value.Allows(workspaceID, version) {
		return domain.ErrNotAllowed
	}
	return nil
}
