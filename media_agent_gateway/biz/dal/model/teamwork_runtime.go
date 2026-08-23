package model

import "time"

type TeamworkTopologyVersion struct {
	TopologyID, WorkspaceID, Status string
	Version                         int64
	UpdatedAt                       time.Time
}

func (TeamworkTopologyVersion) TableName() string {
	return "media_agent_gateway.teamwork_topology_versions"
}

type TeamworkTopologyInbox struct {
	EventID, WorkspaceID, TopologyID, Status string
	TopologyVersion                          int64
	ProcessedAt                              time.Time
}

func (TeamworkTopologyInbox) TableName() string {
	return "media_agent_gateway.teamwork_topology_inbox"
}
