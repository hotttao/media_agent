package model

import (
	"gorm.io/datatypes"
	"time"
)

type Provider struct {
	ID, Name, ProviderType, Endpoint, Status string
	Capabilities                             datatypes.JSON
	Version                                  int64
	CreatedAt, UpdatedAt                     time.Time
}

func (Provider) TableName() string { return "providers" }
