package teamwork

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/teamwork"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Create(ctx context.Context, v *d.Topology) error {
	return dal.FromContext(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&model.TeamworkTopology{ID: v.ID, WorkspaceID: v.WorkspaceID, Name: v.Name, EntryMemberID: v.EntryMemberID, Version: v.Version, Status: string(v.Status), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}).Error; e != nil {
			return e
		}
		for _, m := range v.Members {
			if e := tx.Create(&model.TeamworkMember{TopologyID: v.ID, MemberID: m.MemberID, AgentConfigID: m.AgentConfigID, Name: m.Name, CreatedAt: v.CreatedAt}).Error; e != nil {
				return e
			}
		}
		for _, e := range v.Edges {
			b, _ := json.Marshal(e.Configuration)
			if err := tx.Create(&model.TeamworkEdge{TopologyID: v.ID, FromMemberID: e.FromMemberID, ToMemberID: e.ToMemberID, Mode: e.Mode, Configuration: b}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *Repository) ListOwned(ctx context.Context, o, w string) (out []d.Topology, err error) {
	var rows []model.TeamworkTopology
	if err = dal.FromContext(ctx, r.db).Table("teamwork_topologies t").Joins("JOIN workspaces w ON w.id=t.workspace_id").Where("t.workspace_id=? AND w.owner_user_id=? AND t.status <> ?", w, o, "deleted").Order("t.updated_at DESC").Find(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		v, e := r.load(ctx, row, o)
		if e != nil {
			return nil, e
		}
		out = append(out, *v)
	}
	return
}
func (r *Repository) GetOwned(ctx context.Context, id, o string) (*d.Topology, error) {
	var row model.TeamworkTopology
	if err := dal.FromContext(ctx, r.db).Where("id=?", id).First(&row).Error; err != nil {
		return nil, d.ErrNotFound
	}
	var ws string
	if err := dal.FromContext(ctx, r.db).Table("workspaces").Where("id=? AND owner_user_id=?", row.WorkspaceID, o).Select("id").Scan(&ws).Error; err != nil || ws == "" {
		return nil, d.ErrNotFound
	}
	return r.load(ctx, row, o)
}
func (r *Repository) Update(ctx context.Context, v *d.Topology, expected int64) error {
	return dal.FromContext(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.TeamworkTopology{}).Where("id=? AND version=?", v.ID, expected).Updates(map[string]any{"name": v.Name, "entry_member_id": v.EntryMemberID, "version": v.Version, "updated_at": v.UpdatedAt})
		if res.RowsAffected == 0 {
			return d.ErrConflict
		}
		if err := tx.Where("topology_id=?", v.ID).Delete(&model.TeamworkEdge{}).Error; err != nil {
			return err
		}
		if err := tx.Where("topology_id=?", v.ID).Delete(&model.TeamworkMember{}).Error; err != nil {
			return err
		}
		for _, m := range v.Members {
			if err := tx.Create(&model.TeamworkMember{TopologyID: v.ID, MemberID: m.MemberID, AgentConfigID: m.AgentConfigID, Name: m.Name, CreatedAt: v.CreatedAt}).Error; err != nil {
				return err
			}
		}
		for _, e := range v.Edges {
			b, _ := json.Marshal(e.Configuration)
			if err := tx.Create(&model.TeamworkEdge{TopologyID: v.ID, FromMemberID: e.FromMemberID, ToMemberID: e.ToMemberID, Mode: e.Mode, Configuration: b}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *Repository) Delete(ctx context.Context, v *d.Topology, expected int64) error {
	res := dal.FromContext(ctx, r.db).Model(&model.TeamworkTopology{}).Where("id=? AND version=?", v.ID, expected).Updates(map[string]any{"status": "deleted", "version": v.Version, "updated_at": v.UpdatedAt})
	if res.RowsAffected == 0 {
		return d.ErrConflict
	}
	return res.Error
}
func (r *Repository) load(ctx context.Context, row model.TeamworkTopology, _ string) (*d.Topology, error) {
	var ms []model.TeamworkMember
	var es []model.TeamworkEdge
	db := dal.FromContext(ctx, r.db)
	if e := db.Where("topology_id=?", row.ID).Find(&ms).Error; e != nil {
		return nil, e
	}
	if e := db.Where("topology_id=?", row.ID).Find(&es).Error; e != nil {
		return nil, e
	}
	v := &d.Topology{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, EntryMemberID: row.EntryMemberID, Version: row.Version, Status: d.Status(row.Status), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	for _, m := range ms {
		v.Members = append(v.Members, d.Member{MemberID: m.MemberID, AgentConfigID: m.AgentConfigID, Name: m.Name})
	}
	for _, e := range es {
		var c map[string]any
		_ = json.Unmarshal(e.Configuration, &c)
		v.Edges = append(v.Edges, d.Edge{FromMemberID: e.FromMemberID, ToMemberID: e.ToMemberID, Mode: e.Mode, Configuration: c})
	}
	return v, nil
}
