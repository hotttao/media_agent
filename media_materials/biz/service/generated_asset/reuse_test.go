package generated_asset

import (
	"errors"
	"testing"

	domain "media_agent/media_materials/biz/domain/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
)

func nativeReusableAsset() *domain.GeneratedAsset {
	return &domain.GeneratedAsset{ID: "gen_asset", OrganizationID: "org", Scope: "media", OwnerUserID: "owner", Origin: "native", JobID: "job", StepRunID: "step", TemplateID: "template", TemplateVersion: 1, AssetKind: "model_image", ObjectPath: "materials/org/media/image.png"}
}
func TestReusableAssetRequiresCurrentOwnedNativeResource(t *testing.T) {
	for name, mutate := range map[string]func(*domain.GeneratedAsset){
		"different owner":                        func(a *domain.GeneratedAsset) { a.OwnerUserID = "other" },
		"different company":                      func(a *domain.GeneratedAsset) { a.OrganizationID = "other" },
		"different scope":                        func(a *domain.GeneratedAsset) { a.Scope = "other" },
		"historical without generating contract": func(a *domain.GeneratedAsset) { a.Origin = "legacy_import" },
		"wrong namespace":                        func(a *domain.GeneratedAsset) { a.ObjectPath = "materials/other/media/image.png" },
		"missing source":                         func(a *domain.GeneratedAsset) { a.StepRunID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			a := nativeReusableAsset()
			mutate(a)
			s := NewService(&importRepo{asset: a}, importProduct{}, importVIP{})
			s.ConfigureHistoryImport(importStorage{err: errors.New("must not resolve inaccessible object")}, importKinds{})
			if _, err := s.VerifyReusable(t.Context(), "gen_asset", "org", "media", "owner"); !errors.Is(err, ErrReusableAssetNotFound) {
				t.Fatal("unavailable source accepted", err)
			}
		})
	}
	record := nativeReusableAsset()
	s := NewService(&importRepo{asset: record}, importProduct{}, importVIP{})
	s.ConfigureHistoryImport(importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42}}, importKinds{})
	if got, err := s.VerifyReusable(t.Context(), record.ID, "org", "media", "owner"); err != nil || got.ID != record.ID {
		t.Fatal("owned accepted resource rejected", err)
	}
	text := *record
	text.ObjectPath = ""
	text.Content = "confirmed caption"
	text.AssetKind = "caption"
	s = NewService(&importRepo{asset: &text}, importProduct{}, importVIP{})
	if _, err := s.VerifyReusable(t.Context(), text.ID, "org", "media", "owner"); err != nil {
		t.Fatal("valid text-only output unexpectedly required storage", err)
	}
}
func TestReusableAssetDistinguishesRemovedObjectFromStorageOutage(t *testing.T) {
	s := NewService(&importRepo{asset: nativeReusableAsset()}, importProduct{}, importVIP{})
	for _, test := range []struct{ dependency, expected error }{{objectstorage.ErrNotFound, ErrReusableAssetNotFound}, {objectstorage.ErrUnavailable, ErrReuseVerificationUnavailable}} {
		s.ConfigureHistoryImport(importStorage{err: test.dependency}, importKinds{})
		if _, err := s.VerifyReusable(t.Context(), "gen_asset", "org", "media", "owner"); !errors.Is(err, test.expected) {
			t.Fatal("cache miss and dependency failure were conflated", err)
		}
	}
}
