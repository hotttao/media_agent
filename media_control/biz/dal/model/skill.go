package model

import "time"

type Skill struct {
	ID, OwnerUserID, Name, ContentPath, Checksum, Status                                         string
	WorkspaceID, OrganizationID                                                                  *string
	Visibility, Description, DraftChecksum, SourceKind, GitURL, GitBranch, GitSubpath, GitCommit string
	EncryptedGitToken                                                                            []byte
	Version, PublishedVersion                                                                    int64
	CreatedAt, UpdatedAt                                                                         time.Time
}

func (Skill) TableName() string { return "skills" }
