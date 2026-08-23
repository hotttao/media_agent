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

type TeamworkTopologyOutbox struct {
	ID, WorkspaceID, TopologyID, EventType, Status string
	TopologyVersion                                int64
	Payload                                        datatypes.JSON
	Attempts                                       int32
	AvailableAt, CreatedAt, UpdatedAt              time.Time
	DeliveredAt                                    *time.Time
	LastError                                      *string
}

func (TeamworkTopologyOutbox) TableName() string { return "teamwork_topology_outbox" }

func (TeamworkEdge) TableName() string { return "teamwork_edges" }
