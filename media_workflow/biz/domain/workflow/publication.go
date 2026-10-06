package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

var ErrPublicationConflict = errors.New("workflow publication conflicts with frozen version")
var ErrPublicationClaimStale = errors.New("workflow publication claim is no longer current")
var ErrWorkflowForbidden = errors.New("workflow operation is not permitted")

type Publication struct {
	ID, WorkflowID, CompilationID, RequestedBy string
	WorkflowVersion                            int
	DefinitionHash                             string
	DefinitionJSON, TaskDefinitionsJSON        json.RawMessage
	PayloadDigest, Status, ClaimToken          string
}

func publicationJSON(raw []byte, limit int) (any, error) {
	if len(raw) == 0 || len(raw) > limit {
		return nil, ErrPublicationConflict
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, ErrPublicationConflict
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrPublicationConflict
	}
	return value, nil
}

func (p Publication) Digest() (string, error) {
	definition, err := publicationJSON(p.DefinitionJSON, 4<<20)
	if err != nil {
		return "", err
	}
	tasks, err := publicationJSON(p.TaskDefinitionsJSON, 1<<20)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal([]any{p.ID, p.WorkflowID, p.WorkflowVersion, p.CompilationID, p.RequestedBy, p.DefinitionHash, definition, tasks})
	if err != nil {
		return "", ErrPublicationConflict
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func publicationIdentity(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n\t")
}

func publicationHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (p Publication) Validate() error {
	if !publicationIdentity(p.ID) || !publicationIdentity(p.WorkflowID) || !publicationIdentity(p.CompilationID) || !publicationIdentity(p.RequestedBy) ||
		p.WorkflowVersion <= 0 || p.WorkflowVersion > 2147483647 || !publicationHash(p.DefinitionHash) || !publicationHash(p.PayloadDigest) {
		return ErrPublicationConflict
	}
	value, err := publicationJSON(p.DefinitionJSON, 4<<20)
	if err != nil {
		return err
	}
	definition, ok := value.(map[string]any)
	if !ok {
		return ErrPublicationConflict
	}
	name, ok := definition["name"].(string)
	if !ok || !publicationIdentity(name) {
		return ErrPublicationConflict
	}
	version, ok := definition["version"].(json.Number)
	if !ok {
		return ErrPublicationConflict
	}
	parsed, err := version.Int64()
	if err != nil || parsed <= 0 || parsed > 2147483647 {
		return ErrPublicationConflict
	}
	value, err = publicationJSON(p.TaskDefinitionsJSON, 1<<20)
	if err != nil {
		return err
	}
	tasks, ok := value.([]any)
	if !ok || len(tasks) == 0 || len(tasks) > 512 {
		return ErrPublicationConflict
	}
	names := map[string]bool{}
	for _, value := range tasks {
		task, ok := value.(map[string]any)
		if !ok {
			return ErrPublicationConflict
		}
		name, ok := task["name"].(string)
		if !ok || !publicationIdentity(name) || names[name] {
			return ErrPublicationConflict
		}
		names[name] = true
	}
	digest, err := p.Digest()
	if err != nil || digest != p.PayloadDigest {
		return ErrPublicationConflict
	}
	return nil
}

type PublicationRepo interface {
	// Caller Tx: Workflow lock, frozen Compilation and intent acceptance,
	// then publishing state are one atomic fact. Existing intent is replayed.
	EnqueuePublication(context.Context, *Compilation, Publication) (*Publication, bool, error)
	GetPublication(context.Context, string, int) (*Publication, error)
	ClaimPublication(context.Context) (*Publication, error)
	// Finish requires caller Tx; release is a single conditional write.
	FinishPublication(context.Context, Publication, string, string) error
	ReleasePublication(context.Context, Publication, string) error
}
