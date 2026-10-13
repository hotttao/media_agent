package model

import "time"

type AgentSession struct {
	ID, UserID, WorkspaceID, AgentID, AgentType, Title, Status, AgentConfigID, MetadataTargetContextVersion string
	EnvironmentID                                                                                           string `gorm:"column:environment_id"`
	ReplicaID                                                                                               string `gorm:"column:replica_id"`
	IncarnationID                                                                                           string `gorm:"column:incarnation_id"`
	BindingPhase                                                                                            string `gorm:"column:binding_phase;default:unbound"`
	TransferID                                                                                              string `gorm:"column:transfer_id"`
	TargetReplicaID                                                                                         string `gorm:"column:target_replica_id"`
	TargetIncarnationID                                                                                     string `gorm:"column:target_incarnation_id"`
	TokenHash                                                                                               string `gorm:"column:token_hash"`
	TokenKeyID                                                                                              string `gorm:"column:token_key_id"`
	BindingVersion                                                                                          int64  `gorm:"column:binding_version"`
	ExecutionEpoch                                                                                          int64  `gorm:"column:execution_epoch;default:1"`
	ExpectedExecutionEpoch                                                                                  int64  `gorm:"column:expected_execution_epoch;default:1"`
	ContextVersion                                                                                          int64
	CreatedAt, UpdatedAt                                                                                    time.Time
}

func (AgentSession) TableName() string { return "media_agent_gateway.agent_sessions" }
