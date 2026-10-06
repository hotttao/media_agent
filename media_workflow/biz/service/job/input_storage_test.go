package job

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "media_agent/media_workflow/biz/domain/job"
)

func TestFreezeInputStorageCopiesAllMediaAndPreservesText(t *testing.T) {
	s := &Service{resumeStorageOrigin: "https://storage.test"}
	var reads int
	s.resumeURLVerifier = func(context.Context, string) error { reads++; return nil }
	raw := "https://storage.test/media/materials/org/image.png?secret=temporary"
	input := domain.JobInput{
		Product:   &domain.ProductData{ID: "p", ImageURL: raw},
		VirtualIP: &domain.VirtualIpData{AvatarURL: raw, FullBodyURL: raw, ThreeViewURL: raw, NineViewURL: raw},
		Materials: []domain.MaterialData{{Name: "scene", URL: raw}, {Name: "movement", Content: "walk"}},
		Assets:    []domain.AssetData{{Name: "frame", URL: raw}},
	}
	frozen, err := s.freezeInputStorage(context.Background(), "org", input)
	if err != nil {
		t.Fatal(err)
	}
	want := "object_path:materials/org/image.png"
	for _, value := range []string{frozen.Product.ImageURL, frozen.VirtualIP.AvatarURL, frozen.VirtualIP.FullBodyURL, frozen.VirtualIP.ThreeViewURL, frozen.VirtualIP.NineViewURL, frozen.Materials[0].URL, frozen.Assets[0].URL} {
		if value != want {
			t.Fatal("temporary media URL was frozen", value)
		}
	}
	if reads != 1 || frozen.Materials[1].URL != "" || frozen.Materials[1].Content != "walk" {
		t.Fatal("duplicate resolution or text-only input changed")
	}
	if input.Product.ImageURL != raw || input.VirtualIP.FullBodyURL != raw || input.Materials[0].URL != raw || input.Assets[0].URL != raw {
		t.Fatal("freezing mutated the caller's bundle")
	}
	inputs := conductorInputs(frozen)
	if inputs["product"].(map[string]any)["asset_ref"] != want || inputs["virtual_ip"].(map[string]any)["asset_ref"] != want {
		t.Fatal("Conductor input lost stable media references")
	}
}

func TestFreezeInputStorageFailsClosedWithoutLeakingURL(t *testing.T) {
	for _, raw := range []string{
		"https://other.test/media/materials/org/image.png?secret=temporary",
		"https://storage.test/media/materials/other/image.png?secret=temporary",
		"https://storage.test/media/materials/org/../other/image.png?secret=temporary",
		"object_path:materials/org/image.png",
	} {
		t.Run(raw, func(t *testing.T) {
			s := &Service{resumeStorageOrigin: "https://storage.test", resumeURLVerifier: func(context.Context, string) error { t.Fatal("untrusted URL reached storage"); return nil }}
			_, err := s.freezeInputStorage(context.Background(), "org", domain.JobInput{Assets: []domain.AssetData{{URL: raw}}})
			if err == nil || strings.Contains(err.Error(), "secret=") {
				t.Fatal("untrusted input accepted or credentials leaked")
			}
		})
	}
	s := &Service{resumeStorageOrigin: "https://storage.test", resumeURLVerifier: func(context.Context, string) error { return errors.New("upstream secret=temporary") }}
	_, err := s.freezeInputStorage(context.Background(), "org", domain.JobInput{Product: &domain.ProductData{ImageURL: "https://storage.test/media/materials/org/image.png?secret=temporary"}})
	if err == nil || strings.Contains(err.Error(), "secret=") {
		t.Fatal("unreadable media accepted or upstream secret leaked")
	}
}
