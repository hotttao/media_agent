package job

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrSubmissionConflict = errors.New("submission key already accepted different parameters")

type SubmissionKey struct {
	SubmittedBy, OrganizationID, WorkspaceID, IdempotencyKey string
}

func (k SubmissionKey) Validate() error {
	if k.SubmittedBy == "" || k.OrganizationID == "" || k.WorkspaceID == "" || len(k.IdempotencyKey) == 0 || len(k.IdempotencyKey) > 160 {
		return errors.New("invalid submission identity")
	}
	for _, ch := range []byte(k.IdempotencyKey) {
		if ch < 33 || ch > 126 {
			return errors.New("invalid submission key")
		}
	}
	return nil
}

// A collision only serializes unrelated intents; the full database primary
// key, never this hash, decides identity and authorization.
func (k SubmissionKey) LockID() int64 {
	encoded, _ := json.Marshal([]string{k.SubmittedBy, k.OrganizationID, k.WorkspaceID, k.IdempotencyKey})
	hash := sha256.Sum256(encoded)
	return int64(binary.BigEndian.Uint64(hash[:8]))
}

type Submission struct {
	SubmissionKey
	RequestDigest, JobID string
}

func (s Submission) Validate() error {
	if err := s.SubmissionKey.Validate(); err != nil {
		return err
	}
	if s.JobID == "" || len(s.RequestDigest) != 64 || strings.ToLower(s.RequestDigest) != s.RequestDigest {
		return errors.New("invalid submission receipt")
	}
	if _, err := hex.DecodeString(s.RequestDigest); err != nil {
		return errors.New("invalid submission digest")
	}
	return nil
}

type SubmissionRepo interface {
	// lock requires the caller transaction and covers absent records too.
	GetSubmission(context.Context, SubmissionKey, bool) (*Submission, error)
	// Save requires the same transaction as the accepted Job and initial start.
	SaveSubmission(context.Context, Submission) error
}
