package media_management

import (
	"context"
	"errors"
	"media_agent/hertz_infra/authn/mcpdelegation"
	contract "media_agent/media_worker/biz/shared/media_management"
	"testing"
)

type objectProbe struct {
	paths  []string
	denied bool
}

func (p *objectProbe) Get(_ context.Context, kind, id string) (map[string]any, error) {
	if p.denied {
		return nil, &Failure{Code: "not_found_or_forbidden", Message: "Denied"}
	}
	switch kind {
	case "products":
		return map[string]any{"id": id, "name": "Product", "scope": "ai_clothing", "images": []any{map[string]any{"object_path": "materials/org/first.png"}, map[string]any{"object_path": "materials/org/main.png", "is_main": true}}}, nil
	case "virtual-ips":
		return map[string]any{"id": id, "scope": "ai_clothing", "avatar_object_path": "materials/org/avatar.png"}, nil
	}
	return nil, errors.New("unexpected resource")
}
func (p *objectProbe) Resolve(_ context.Context, path string) (map[string]any, error) {
	p.paths = append(p.paths, path)
	if p.denied {
		return nil, &Failure{Code: "permission_denied", Message: "Denied"}
	}
	return map[string]any{"url": "https://storage/fresh", "content_type": "image/png"}, nil
}
func TestProductAndVirtualIPSnapshotsResolveAuthorizedObjectPaths(t *testing.T) {
	objects := &objectProbe{}
	o := Operations{Objects: objects}
	input, err := o.resolveInput(context.Background(), contract.JobInput{ProductID: "product", VirtualIPID: "ip"}, "ai_clothing")
	if err != nil || input.Product["image_url"] != "https://storage/fresh" || input.VirtualIP["avatar_url"] != "https://storage/fresh" || len(objects.paths) != 2 || objects.paths[0] != "materials/org/main.png" {
		t.Fatalf("bad snapshot %+v paths=%v %v", input, objects.paths, err)
	}
	objects.denied = true
	if _, err := o.resolveInput(context.Background(), contract.JobInput{ProductID: "private"}, "ai_clothing"); err == nil {
		t.Fatal("private product snapshot accepted")
	}
}
func TestCandidateURLsAreResolvedAndNeverReturnedStale(t *testing.T) {
	ctx := mcpdelegation.WithPrincipal(context.Background(), mcpdelegation.Principal{OrganizationID: "org"})
	objects := &objectProbe{}
	o := Operations{Objects: objects}
	for _, test := range []struct {
		old       string
		available bool
	}{
		{"https://old-storage/media/materials/org/video.mp4?expired-secret", true},
		{"object_path:materials/org/video.mp4", true},
		{"https://external/video.mp4", false},
		{"https://old-storage/media/materials/other/video.mp4", false},
		{"https://old-storage/media/materials/org/../other/video.mp4", false},
	} {
		candidate := map[string]any{"url": test.old, "index": 1}
		o.candidateArtifact(ctx, candidate)
		if _, ok := candidate["url"]; ok {
			t.Fatal("stale URL disclosed")
		}
		if (candidate["artifact_access"] == "available") != test.available {
			t.Fatalf("wrong candidate access %+v", candidate)
		}
	}
	objects.denied = true
	candidate := map[string]any{"url": "object_path:materials/org/video.mp4"}
	o.candidateArtifact(ctx, candidate)
	if candidate["artifact_access"] != "denied" {
		t.Fatal("resource denial not preserved")
	}
}

func TestImageBindingsRejectConstantURLsAndTextMedia(t *testing.T) {
	template := map[string]any{"invocation_mapping": map[string]any{"images": map[string]any{"op": "list", "items": []any{map[string]any{"op": "asset_ref", "slot": "image"}}}}}
	if err := validateImageBindings(template, map[string]any{"image": map[string]any{"source": 3, "const_value": "https://external/image.png"}}, nil); err == nil {
		t.Fatal("constant download URL accepted")
	}
	bindings := map[string]any{"image": map[string]any{"source": 1, "ref": "input-image"}}
	input := contract.JobInput{Assets: []contract.MediaInput{{Name: "input-image", Content: "https://external/image.png"}}}
	if err := validateImageBindings(template, bindings, &input); err == nil {
		t.Fatal("content replaced image reference")
	}
	input.Assets[0].Content = ""
	input.Assets[0].ObjectPath = "materials/org/input.png"
	if err := validateImageBindings(template, bindings, &input); err != nil {
		t.Fatal(err)
	}
}

func TestInputResourceCannotCrossWorkflowBusinessScope(t *testing.T) {
	o := Operations{Objects: &objectProbe{}}
	for _, input := range []contract.JobInput{{ProductID: "product"}, {VirtualIPID: "ip"}} {
		if _, err := o.resolveInput(context.Background(), input, "other_scope"); err == nil {
			t.Fatal("cross-scope resource accepted")
		}
	}
}
