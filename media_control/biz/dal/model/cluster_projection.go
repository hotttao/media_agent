package model

import "time"

type ClusterProjection struct {
	ClusterID  string
	Status     string
	Capacity   string `gorm:"type:jsonb"`
	ObservedAt time.Time
}

func (ClusterProjection) TableName() string { return "cluster_projections" }
