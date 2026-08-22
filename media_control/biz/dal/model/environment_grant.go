package model

import "time"

type EnvironmentGrant struct {
	ID, EnvironmentID, SubjectID, SubjectType, Permission, Status string
	CreatedAt, RevokedAt                                          *time.Time
}

func (EnvironmentGrant) TableName() string { return "environment_grants" }
