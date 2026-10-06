package generated_asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrResultConflict = errors.New("generation result registration conflict")

type ResultReceipt struct {
	StepRunID, ResultKey, AssetID, RequestDigest string
}

func (r ResultReceipt) Validate() error {
	if ValidateResultIdentity(r.StepRunID, r.ResultKey) != nil ||
		strings.TrimSpace(r.AssetID) == "" || strings.TrimSpace(r.AssetID) != r.AssetID || len(r.AssetID) > 256 || !resultHex(r.RequestDigest) {
		return ErrResultConflict
	}
	return nil
}

func ValidateResultIdentity(stepRunID, resultKey string) error {
	if strings.TrimSpace(stepRunID) == "" || strings.TrimSpace(stepRunID) != stepRunID || len(stepRunID) > 256 || !resultHex(resultKey) {
		return ErrResultConflict
	}
	return nil
}

func resultHex(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type ResultRepo interface {
	// lock=true requires a caller transaction and serializes absent keys too.
	GetResult(ctx context.Context, stepRunID, resultKey string, lock bool) (*ResultReceipt, error)
	SaveResult(ctx context.Context, receipt ResultReceipt) error
}

// ResultDigest receives an authorized, normalized native resource. Transport
// signatures and mutable timestamps never determine the accepted result.
func ResultDigest(a *GeneratedAsset) (string, error) {
	if a == nil || a.Origin != "native" || a.OrganizationID == "" || a.Scope == "" || a.AssetKind == "" || a.OwnerUserID == "" ||
		a.JobID == "" || a.StepRunID == "" || a.TemplateID == "" || a.TemplateVersion <= 0 || a.Name == "" ||
		a.ObjectPath == "" && (a.URL != "" || strings.TrimSpace(a.Content) == "") {
		return "", ErrResultConflict
	}
	encoded, err := json.Marshal([]any{a.OrganizationID, a.Scope, a.AssetKind, a.Name, a.OwnerUserID, string(a.Visibility), a.TeamID,
		a.JobID, a.StepRunID, a.TemplateID, a.TemplateVersion, a.ObjectPath, a.Content, a.SourceProductID, a.SourceVirtualIPID})
	if err != nil {
		return "", ErrResultConflict
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
