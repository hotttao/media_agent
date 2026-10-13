package agent_instance

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("agent instance not found")
var ErrIncarnationConflict = errors.New("native generation incarnation conflict")
var ErrConflict = errors.New("instance registration conflict")
var ErrInvalid = errors.New("invalid verified instance")

type Instance struct {
	EnvironmentID, WorkspaceID, AgentConfigID, AgentType, ReplicaID, IncarnationID, ResourceUID, ResourceGeneration, ResourceName, Namespace, Endpoint, CredentialHash, CredentialKeyID, Admission, DrainOperationID string
	Version                                                                                                                                                                                                          int64
	UpdatedAt                                                                                                                                                                                                        time.Time
}
type Repository interface {
	Get(context.Context, string, string, string, string, bool) (*Instance, error)
	Historical(context.Context, string, string, string, string, string) (*Instance, error)
	List(context.Context, string, string, string) ([]Instance, error)
	Save(context.Context, *Instance, int64) error
	Drain(context.Context, *Instance, string) error
}
type ResourceLocator struct{ EnvironmentID, WorkspaceID, AgentConfigID, ReplicaID, Namespace, ResourceName, ResourceUID string }
type ResourceFacts struct {
	Locator                         ResourceLocator
	Generation, Endpoint, AgentType string
	Eligible, Ready, Running        bool
}
type ResourceAuthority interface {
	Resolve(context.Context, ResourceLocator, string) (ResourceFacts, error)
	Observe(context.Context, Instance) (ResourceFacts, error)
	Terminated(context.Context, Instance) (bool, error)
}
