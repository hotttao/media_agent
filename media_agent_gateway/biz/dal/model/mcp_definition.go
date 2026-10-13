package model

import "time"

type MCPDefinition struct {
	ID                                                           string `gorm:"primaryKey"`
	ServiceName, Endpoint, Transport, AuthenticationType, Status string
	Version                                                      int64
	CreatedAt, UpdatedAt                                         time.Time
}

func (MCPDefinition) TableName() string { return "media_agent_gateway.mcp_definitions" }
