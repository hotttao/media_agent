package agent_configuration

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("metadata_not_found")
var ErrConflict = errors.New("metadata_version_conflict")
var ErrForbidden = errors.New("metadata_permission_denied")
var ErrInvalid = errors.New("metadata_invalid")

type ResourceReference struct {
	Kind, ID, Checksum string
	Version            int64
}
type ResourceVersion struct {
	ResourceReference
	OwnerUserID, WorkspaceID string
	Payload                  []byte
}
type Resource struct {
	ResourceReference
	Name, ContentRef, Content string
}
type Snapshot struct {
	PlatformPromptTemplateVersion                                                 string
	WorkspaceID, AgentConfigID, TargetContextVersion, AgentType, ConfiguredPrompt string
	SelectionRevision, ContextVersion                                             int64
	Resources                                                                     []Resource
}
type Release struct {
	Snapshot
	RequestHash, IdempotencyKey, CreatorID string
	Manifest                               []byte
	CreatedAt                              time.Time
}
type ApplicationResult struct {
	ReportID, WorkspaceID, AgentConfigID, TargetContextVersion, ReplicaID, IncarnationID, Status, AppliedContextVersion, ErrorCode string
	SelectionRevision                                                                                                              int64
}
type Instance struct{ WorkspaceID, AgentConfigID, ReplicaID, IncarnationID, Admission string }
type Notice struct {
	EventID, RoutingKey, LeaseToken string
	Payload                         []byte
	Attempts                        int
}
type Repository interface {
	Resource(context.Context, ResourceReference) (*ResourceVersion, error)
	FindByKey(context.Context, string, string) (*Release, error)
	Current(context.Context, string, string) (*Release, error)
	Find(context.Context, string, string, string) (*Release, error)
	SaveSelection(context.Context, *Release, int64, Notice) error
	SaveReport(context.Context, ApplicationResult, string) error
	Reports(context.Context, string, int64) ([]ApplicationResult, error)
	ClaimNotices(context.Context, int) ([]Notice, error)
	FinishNotice(context.Context, Notice, bool) error
}
