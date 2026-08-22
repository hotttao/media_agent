package model

import "time"

type CredentialReference struct {
	ID, OwnerUserID, Name, ProviderID, SecretRef, Status string
	WorkspaceID                                          *string
	Version                                              int64
	CreatedAt, UpdatedAt                                 time.Time
}

func (CredentialReference) TableName() string { return "credential_references" }
