package model

type ApplicationBinding struct{ ApplicationID, UserID, TeamID, Name, WorkspaceID, Digest string }

func (ApplicationBinding) TableName() string { return "application_agent_bindings" }
