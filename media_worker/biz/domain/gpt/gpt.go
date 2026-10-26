// Package gpt owns browser image-generation facts. It is independent of Studio.
package gpt

import "context"

const TextCapability = "gpt.t2i.text_prompt"
const ReferenceCapability = "gpt.i2i.multi_image_prompt"

type Request struct {
	Key, Prompt, Size, WebModel string
	Images                      []LocalImage
}
type LocalImage struct {
	Path, ChecksumSHA256 string
	SizeBytes            int64
}
type Reference string
type LookupState string

const (
	Found     LookupState = "found"
	Absent    LookupState = "absent"
	Uncertain LookupState = "uncertain"
)

type LookupResult struct {
	State     LookupState
	Reference Reference
}
type State string

const (
	Generating State = "generating"
	NeedsHuman State = "needs_human"
	Succeeded  State = "succeeded"
)

type Artifact struct{ ID, MediaType string }
type Observation struct {
	State     State
	Artifacts []Artifact
	Reason    string
}
type Download struct {
	ArtifactID, LocalPath, ChecksumSHA256 string
	SizeBytes                             int64
}
type Generator interface {
	Submit(context.Context, Request) (Reference, error)
	Lookup(context.Context, string) (LookupResult, error)
	Observe(context.Context, Reference) (Observation, error)
	FetchArtifacts(context.Context, Reference, []Artifact) ([]Download, error)
}

// Job envelope inputs and signed URLs are not part of the durable generation.
type Checkpoint struct {
	DispatchID        string
	Attempt           int
	CapabilityKey     string
	CapabilityVersion int
	TraceParent       string
	Request           Request
	Reference         Reference
	Artifacts         []UploadedArtifact
	Closed            bool
}
type UploadedArtifact struct {
	Download
	ObjectPath string
}
type Repository interface {
	Save(context.Context, Checkpoint) error
	Load(context.Context, string, int) (*Checkpoint, error)
	Pending(context.Context) ([]Checkpoint, error)
}
