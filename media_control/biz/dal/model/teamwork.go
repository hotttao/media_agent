package model

import (
	"gorm.io/datatypes"
	"time"
)

type TeamworkTopology struct {
	ID, WorkspaceID, Name, EntryMemberID string
	Version                              int64
	Status                               string
	CreatedAt, UpdatedAt                 time.Time
}

func (TeamworkTopology) TableName() string { return "teamwork_topologies" }

type TeamworkMember struct {
	TopologyID, MemberID, AgentConfigID, Name string
	CreatedAt                                 time.Time
}

func (TeamworkMember) TableName() string { return "teamwork_members" }

type TeamworkEdge struct {
	TopologyID, FromMemberID, ToMemberID, Mode string
	Configuration                              datatypes.JSON
}

func (TeamworkEdge) TableName() string { return "teamwork_edges" }
