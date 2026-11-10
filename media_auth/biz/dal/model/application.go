package model

import (
	"github.com/lib/pq"
	"time"
)

type Application struct {
	ID, Name, SecretHash, Status, CreatedBy string
	CredentialVersion                       int64
	CreatedAt                               time.Time
}

func (Application) TableName() string { return "media_auth.applications" }

type ApplicationCredential struct {
	TeamID, OrganizationID                       string
	Scopes                                       pq.StringArray `gorm:"type:text[]"`
	TokenHash, ApplicationID, UserID, ExternalID string
	ApplicationVersion, AuthVersion              int64
	ExpiresAt                                    time.Time
}

func (ApplicationCredential) TableName() string { return "media_auth.application_access_credentials" }

type ApplicationTeamGrant struct {
	ApplicationID, TeamID, OrganizationID string
	Scopes                                pq.StringArray `gorm:"type:text[]"`
}

func (ApplicationTeamGrant) TableName() string { return "media_auth.application_team_grants" }

type ApplicationMCPTarget struct{ DefinitionID, ApplicationID, Origin string }

func (ApplicationMCPTarget) TableName() string { return "media_auth.application_mcp_targets" }
