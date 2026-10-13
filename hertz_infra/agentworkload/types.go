// Package agentworkload defines the native resource contract shared by deployers
// and resource controllers. It contains no authorization or Session rules.
package agentworkload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
)

const APIVersion = "runtime.media-agent.io/v1alpha1"
const Group = "runtime.media-agent.io"
const WorkspaceLabel = Group + "/workspace-id"
const ConfigLabel = Group + "/config-id"
const EnvironmentLabel = Group + "/environment-id"
const AgentTypeLabel = Group + "/agent-type"
const TargetVersionAnnotation = Group + "/control-version"
const DrainAnnotation = Group + "/drain-operation"
const PendingReplicasAnnotation = Group + "/pending-replicas"
const PendingVersionAnnotation = Group + "/pending-version"

var ErrInvalid = errors.New("invalid native workload specification")
var ErrConflict = errors.New("native resource belongs to another workload")
var id = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
var name = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,62}$`)

type Spec struct {
	SessionExchange  string `json:"sessionExchange,omitempty"`
	MetadataExchange string `json:"metadataExchange,omitempty"`
	Driver           string `json:"driver"`
	Namespace        string `json:"namespace"`
	WorkspaceID      string `json:"workspaceId"`
	AgentConfigID    string `json:"agentConfigId"`
	EnvironmentID    string `json:"environmentId"`
	AgentType        string `json:"agentType"`
	Image            string `json:"image"`
	Replicas         int32  `json:"replicas"`
	Port             int32  `json:"port"`
	ClaimName        string `json:"claimName"`
	StorageClass     string `json:"storageClass"`
	StorageSize      string `json:"storageSize"`
	CPU              string `json:"cpu"`
	Memory           string `json:"memory"`
	GatewayURL       string `json:"gatewayUrl"`
	IdentitySecret   string `json:"identitySecret"`
	SharedSecret     string `json:"sharedSecret"`
}

func Name(environment, workspace string) string {
	sum := sha256.Sum256([]byte(environment + "\x00" + workspace))
	return "agent-" + hex.EncodeToString(sum[:12])
}
func (s Spec) Labels() map[string]string {
	return map[string]string{WorkspaceLabel: s.WorkspaceID, ConfigLabel: s.AgentConfigID, EnvironmentLabel: s.EnvironmentID, AgentTypeLabel: s.AgentType}
}
func (s Spec) Validate() error {
	u, e := url.Parse(s.GatewayURL)
	if len(s.SessionExchange) > 255 || len(s.MetadataExchange) > 255 || s.Driver != "kubernetes" && s.Driver != "docker" || s.Driver == "kubernetes" && !name.MatchString(s.Namespace) || !id.MatchString(s.WorkspaceID) || !id.MatchString(s.AgentConfigID) || !id.MatchString(s.EnvironmentID) || s.AgentType != "codex" && s.AgentType != "claude-code" && s.AgentType != "goose" || s.Image == "" || s.Replicas < 0 || s.Replicas > 64 || s.Port < 1 || s.Port > 65535 || !name.MatchString(s.ClaimName) || !name.MatchString(s.IdentitySecret) || !name.MatchString(s.SharedSecret) || e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrInvalid
	}
	return nil
}
func (s Spec) Object() map[string]any {
	body, _ := json.Marshal(s)
	result := map[string]any{}
	json.Unmarshal(body, &result)
	result["replicas"] = int64(s.Replicas)
	result["port"] = int64(s.Port)
	return result
}
func Decode(value any) (Spec, error) {
	body, e := json.Marshal(value)
	if e != nil {
		return Spec{}, ErrInvalid
	}
	var s Spec
	e = json.Unmarshal(body, &s)
	if e != nil {
		return s, ErrInvalid
	}
	return s, s.Validate()
}
