package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"strings"
	"time"
)

var ErrReuseEvidenceConflict = errors.New("generation reuse evidence conflict")

type ReuseScope struct {
	OrganizationID string
	WorkspaceID    string
	OwnerUserID    string
	Scope          string
}

func (s ReuseScope) Valid() bool {
	return s.OrganizationID != "" && s.WorkspaceID != "" && s.OwnerUserID != "" && s.Scope != "" && !strings.ContainsAny(s.OrganizationID+s.Scope, "/\\")
}

// ReuseEvidence is an immutable derived record of a genuinely accepted
// generation. It cannot schedule a Task or mutate the source StepRun/Asset.
type ReuseEvidence struct {
	ID              string
	Boundary        ReuseScope
	Fingerprint     string
	SourceJobID     string
	SourceStepRunID string
	SourceResultKey string
	AssetID         string
	ObjectPath      string
	Content         string
	AssetKind       string
	CreatedAt       time.Time
}

func ReuseEvidenceID(stepRunID, resultKey string) string {
	value := sha256.Sum256([]byte(stepRunID + "\x00" + resultKey))
	return "reuse_" + hex.EncodeToString(value[:])
}

func (e ReuseEvidence) Validate() error {
	decoded, err := hex.DecodeString(e.Fingerprint)
	if !e.Boundary.Valid() || err != nil || len(decoded) != sha256.Size || strings.ToLower(e.Fingerprint) != e.Fingerprint || e.SourceJobID == "" || e.SourceStepRunID == "" || e.SourceResultKey == "" || e.ID != ReuseEvidenceID(e.SourceStepRunID, e.SourceResultKey) || !strings.HasPrefix(e.AssetID, "gen_") || e.AssetKind == "" || (e.ObjectPath == "" && strings.TrimSpace(e.Content) == "") {
		return ErrReuseEvidenceConflict
	}
	if e.ObjectPath != "" && (!strings.HasPrefix(e.ObjectPath, "materials/"+e.Boundary.OrganizationID+"/"+e.Boundary.Scope+"/") || path.Clean(e.ObjectPath) != e.ObjectPath || strings.ContainsAny(e.ObjectPath, "\\?#\x00")) {
		return ErrReuseEvidenceConflict
	}
	return nil
}

func (e ReuseEvidence) Same(other ReuseEvidence) bool {
	// Creation timestamp is assigned once by storage, not part of replay.
	e.CreatedAt, other.CreatedAt = time.Time{}, time.Time{}
	return e == other
}

type ResultReuseRepo interface {
	Find(context.Context, ReuseScope, string) ([]ReuseEvidence, error)
	Save(context.Context, ReuseEvidence) error
}
