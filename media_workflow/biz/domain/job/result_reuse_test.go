package job

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validEvidence() ReuseEvidence {
	return ReuseEvidence{ID: ReuseEvidenceID("step", "result"), Boundary: ReuseScope{OrganizationID: "org", WorkspaceID: "workspace", OwnerUserID: "owner", Scope: "media"}, Fingerprint: strings.Repeat("a", 64), SourceJobID: "job", SourceStepRunID: "step", SourceResultKey: "result", AssetID: "gen_result", ObjectPath: "materials/org/media/result.png", AssetKind: "model_image"}
}
func TestReuseEvidenceCannotCrossNamespaceOrInventResult(t *testing.T) {
	value := validEvidence()
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ReuseEvidence){func(e *ReuseEvidence) { e.Boundary.OrganizationID = "other" }, func(e *ReuseEvidence) { e.SourceResultKey = "new" }, func(e *ReuseEvidence) { e.Fingerprint = "ETag" }, func(e *ReuseEvidence) { e.ObjectPath = "https://provider.test/result.png" }, func(e *ReuseEvidence) { e.Boundary.WorkspaceID = "" }} {
		changed := value
		mutate(&changed)
		if !errors.Is(changed.Validate(), ErrReuseEvidenceConflict) {
			t.Fatal("invalid reuse evidence accepted")
		}
	}
	text := value
	text.ObjectPath = ""
	text.Content = "caption"
	text.AssetKind = "caption"
	if err := text.Validate(); err != nil {
		t.Fatal("text-only result rejected", err)
	}
}
func TestReuseEvidenceReplayDoesNotChangeResult(t *testing.T) {
	a := validEvidence()
	b := a
	b.CreatedAt = time.Unix(10, 0)
	if !a.Same(b) {
		t.Fatal("storage timestamp broke an identical replay")
	}
	b.AssetID = "gen_different"
	if a.Same(b) {
		t.Fatal("same key with a different generated asset accepted")
	}
}
