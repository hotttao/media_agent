// Package media_management defines application commands shared with API adapters.
// These are not persisted resources or generated API DTOs.
package media_management

import "context"

type Slot struct {
	Name, ResourceType, AssetKind string
	Required                      bool
	DisplayOrder                  int
}
type Expression struct {
	Op, Slot string
	Items    []Expression
}
type Binding struct {
	Source          int
	Ref, ConstValue string
}
type Step struct {
	Key, Name, TemplateID, ApprovalPolicy, ReusePolicy string
	Order, TemplateVersion                             int
	Bindings                                           map[string]Binding
}
type MediaInput struct{ Name, AssetKind, ObjectPath, Content, URL string }
type JobInput struct {
	Product, VirtualIP     map[string]any
	ProductID, VirtualIPID string
	Assets, Materials      []MediaInput
}
type Params struct {
	ID, Scope, Name, Description, Visibility, Status string
	Page, PageSize, Version                          int
	PromptBody, OutputAssetKind, CapabilityKey       string
	CapabilityVersion, CandidateCount                int
	NeedsConfirm                                     bool
	Inputs                                           []Slot
	InvocationMapping                                map[string]Expression
	Steps                                            []Step
	OutputStepKey                                    string
	WorkflowID, IdempotencyKey                       string
	WorkflowVersion                                  int
	Input                                            JobInput
	Provider, GenerationMethod, GenerationType       string
}
type API interface {
	Execute(context.Context, string, Params) (map[string]any, error)
}

// Failure is an application outcome, not an HTTP status or SDK error.
type Failure struct {
	Code, Message                string
	Unknown, NotDispatched, Stop bool
}

func (e *Failure) Error() string { return e.Message }

// Credentials are short-lived, target-specific assertions, carried only in memory.
type Credentials struct{ Workflow, Materials string }
type credentialsKey struct{}

func WithCredentials(ctx context.Context, c Credentials) context.Context {
	return context.WithValue(ctx, credentialsKey{}, c)
}
func CredentialsFromContext(ctx context.Context) Credentials {
	c, _ := ctx.Value(credentialsKey{}).(Credentials)
	return c
}
