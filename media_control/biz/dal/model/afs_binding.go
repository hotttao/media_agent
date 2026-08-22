package model

import "time"

type AfsBinding struct {
	ID, WorkspaceID, ExternalRef, DesiredState, ObservedState string
	Version                                                   int64
	CreatedAt, UpdatedAt                                      time.Time
}

func (AfsBinding) TableName() string { return "afs_bindings" }

type AfsBindingMember struct {
	BindingID, SubjectID, SubjectType, ExternalMemberRef, Status string
	UpdatedAt                                                    time.Time
}

func (AfsBindingMember) TableName() string { return "afs_binding_members" }
