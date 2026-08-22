package model

import (
	"gorm.io/datatypes"
	"time"
)

type MemoryStore struct {
	ID, WorkspaceID, OwnerUserID, Name, StoreType string
	Configuration                                 datatypes.JSON
	CredentialReferenceID                         *string
	Status                                        string
	Version                                       int64
	CreatedAt, UpdatedAt                          time.Time
}

func (MemoryStore) TableName() string { return "memory_stores" }
