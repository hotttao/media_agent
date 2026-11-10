package contract

type UserProfileInput struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}
type UserProfile struct {
	ApplicationID string `json:"application_id"`
	ID            string `json:"id"`
	ExternalID    string `json:"external_id"`
	DisplayName   string `json:"display_name"`
	Email         string `json:"email"`
}
type AgentConfiguration struct {
	DisplayName       string `json:"display_name"`
	AgentType         string `json:"agent_type"`
	APIKey            string `json:"api_key,omitempty"`
	ModelConnectionID string `json:"model_connection_id,omitempty"`
	ModelProtocol     string `json:"model_protocol,omitempty"`
	ModelName         string `json:"model_name,omitempty"`
	ModelBaseURL      string `json:"model_base_url,omitempty"`
}
type Workspace struct {
	ID                string `json:"id"`
	OrganizationID    string `json:"organization_id"`
	TeamID            string `json:"team_id"`
	CreatedBy         string `json:"created_by"`
	Name              string `json:"name"`
	AgentType         string `json:"agent_type"`
	ModelConnectionID string `json:"model_connection_id"`
}
type ModelConnectionInput struct {
	Name     string `json:"name"`
	AuthJSON string `json:"auth_json"`
}
type ModelConnectionUpdate struct {
	Name            string `json:"name"`
	AuthJSON        string `json:"auth_json,omitempty"`
	ExpectedVersion int64  `json:"expected_version"`
}
type ModelConnection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Status    string `json:"status"`
	Version   int64  `json:"version"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}
type Session struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	AgentID        string `json:"agent_id"`
	Status         string `json:"status"`
	Title          string `json:"title"`
	AgentType      string `json:"agent_type"`
	ChatStatus     string `json:"chat_status"`
	RuntimeStatus  string `json:"runtime_status"`
	ContextVersion int64  `json:"context_version"`
	ExecutionEpoch int64  `json:"execution_epoch"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}
type Message struct {
	ID          string `json:"id"`
	TurnID      string `json:"turn_id"`
	Role        string `json:"role"`
	ContentJSON string `json:"content_json"`
	Sequence    int64  `json:"sequence"`
	CreatedAt   int64  `json:"created_at"`
}
type TurnResult struct {
	TurnID string `json:"turn_id"`
	Status string `json:"status"`
}
type AgentRequest struct {
	RequestID    string `json:"request_id"`
	TurnID       string `json:"turn_id"`
	RequestType  string `json:"request_type"`
	PayloadJSON  string `json:"payload_json"`
	Status       string `json:"status"`
	ResponseJSON string `json:"response_json"`
	CreatedAt    int64  `json:"created_at"`
	ResolvedAt   int64  `json:"resolved_at"`
}
