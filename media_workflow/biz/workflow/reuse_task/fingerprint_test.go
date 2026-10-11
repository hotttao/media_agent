package reuse_task

import (
	"context"
	"errors"
	"strings"
	"testing"

	capability "media_agent/media_workflow/biz/domain/capability"
)

type digestResolver struct{ err error }

func (r digestResolver) Checksum(_ context.Context, _ string, ref string, _ capability.MediaType) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if strings.Contains(ref, "person") {
		return strings.Repeat("a", 64), nil
	}
	if strings.Contains(ref, "garment") {
		return strings.Repeat("b", 64), nil
	}
	return "", nil
}
func fingerprintIdentity() Identity {
	return Identity{TemplateID: "tpl", TemplateVersion: 1, CapabilityKey: "jimeng.image", CapabilityVersion: 1, CandidateCount: 1, OutputAssetKind: "model_image", Inputs: []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}, {Name: "images", Type: capability.InputResourceRefList, Required: true, MediaType: capability.MediaImage, MinItems: 1}}}
}
func TestFingerprintUsesContentNotSignedURLOrDelivery(t *testing.T) {
	ctx := context.Background()
	id := fingerprintIdentity()
	a := map[string]any{"prompt": "wear the dress", "images": []string{"person?signature=old", "garment?signature=old"}, "job_id": "one", "step_key": "image"}
	b := map[string]any{"step_key": "different-node", "job_id": "two", "images": []any{"person?signature=new", "garment?signature=new"}, "prompt": "wear the dress"}
	first, err := BuildFingerprint(ctx, "org", id, a, digestResolver{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildFingerprint(ctx, "org", id, b, digestResolver{})
	if err != nil || first != second {
		t.Fatal("equivalent real inputs did not reuse identity", err)
	}
	for name, changed := range map[string]map[string]any{
		"media order": {"prompt": "wear the dress", "images": []string{"garment", "person"}},
		"prompt":      {"prompt": "different style", "images": []string{"person", "garment"}},
	} {
		t.Run(name, func(t *testing.T) {
			value, err := BuildFingerprint(ctx, "org", id, changed, digestResolver{})
			if err != nil || value == first {
				t.Fatal("different generation inputs collided", err)
			}
		})
	}
	for _, changed := range []Identity{func() Identity { v := id; v.TemplateVersion++; return v }(), func() Identity { v := id; v.CapabilityVersion++; return v }(), func() Identity { v := id; v.OutputAssetKind = "other"; return v }()} {
		value, err := BuildFingerprint(ctx, "org", changed, a, digestResolver{})
		if err != nil || value == first {
			t.Fatal("different frozen contract collided", err)
		}
	}
}
func TestFingerprintCannotGuessMissingDigestOrIgnoreProviderOptions(t *testing.T) {
	id := fingerprintIdentity()
	input := map[string]any{"prompt": "hello", "images": []string{"missing"}}
	if _, err := BuildFingerprint(context.Background(), "org", id, input, digestResolver{}); !errors.Is(err, ErrDigestUnavailable) {
		t.Fatal("missing digest was treated as a usable key", err)
	}
	transient := errors.New("storage unavailable")
	if _, err := BuildFingerprint(context.Background(), "org", id, input, digestResolver{err: transient}); !errors.Is(err, transient) {
		t.Fatal("storage failure became a cache miss", err)
	}
	input["images"] = []string{"person"}
	input["unexpected_provider_parameter"] = "high"
	if _, err := BuildFingerprint(context.Background(), "org", id, input, digestResolver{}); !errors.Is(err, ErrInvalidFingerprintInput) {
		t.Fatal("undeclared provider parameter was silently ignored", err)
	}
}
func TestFingerprintSupportsTextOnlyAndRetainsOptionalNull(t *testing.T) {
	id := fingerprintIdentity()
	id.Inputs = []capability.Input{{Name: "prompt", Type: capability.InputString, Required: true}, {Name: "style", Type: capability.InputString}}
	first, err := BuildFingerprint(context.Background(), "org", id, map[string]any{"prompt": "walk towards camera"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildFingerprint(context.Background(), "org", id, map[string]any{"prompt": "walk towards camera", "style": nil}, nil)
	if err != nil || first == second {
		t.Fatal("absent and explicit optional null were merged", err)
	}
}
