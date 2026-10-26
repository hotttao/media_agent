// Package doubao defines provider-independent generation facts and ports.
package doubao

import "context"

type MediaKind string

const (
	Image MediaKind = "IMAGE"
	Video MediaKind = "VIDEO"
)

// Request contains a stable generation key and an already rendered prompt.
// Images are ordered local files. Signed input URLs must not be persisted here.
type Request struct {
	Key             string
	Kind            MediaKind
	Prompt          string
	Images          []LocalImage
	Model           string
	DurationSeconds int
	AspectRatio     string
}
type LocalImage struct {
	Path      string
	MediaType string
}

// Reference is opaque to callers; only its issuing adapter interprets it.
type Reference string
type LookupState string

const (
	Found     LookupState = "found"
	Absent    LookupState = "absent"
	Uncertain LookupState = "uncertain"
)

// Absent is an authoritative negative. Timeout/ambiguous submission is Uncertain,
// never permission to submit a second generation.
type LookupResult struct {
	State     LookupState
	Reference Reference
}
type State string

const (
	Accepted   State = "accepted"
	Generating State = "generating"
	NeedsHuman State = "needs_human"
	Succeeded  State = "succeeded"
	Failed     State = "failed"
)

type Artifact struct {
	ID        string
	Kind      MediaKind
	MediaType string
}
type Observation struct {
	State     State
	Artifacts []Artifact
	Reason    string
}

// Download binds each exact requested artifact ID to its completed local file.
type Download struct {
	ChecksumSHA256 string
	SizeBytes      int64
	ArtifactID     string
	LocalPath      string
}
type Generator interface {
	Submit(context.Context, Request) (Reference, error)
	Lookup(context.Context, string) (LookupResult, error)
	Observe(context.Context, Reference) (Observation, error)
	FetchArtifacts(context.Context, Reference, []Artifact) ([]Download, error)
}
