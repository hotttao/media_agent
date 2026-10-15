package plugin_binding

type Workspace struct{ ID, OrganizationID, AgentConfigID string }
type Definition struct {
	ID, OrganizationID, Endpoint, Transport, AuthenticationType, Status string
	Version                                                             int64
}
type Resource struct {
	Kind, ID, Checksum string
	Version            int64
}
type Release struct {
	WorkspaceID, AgentConfigID, Target string
	ContextVersion                     int64
	Resources                          []Resource
}
