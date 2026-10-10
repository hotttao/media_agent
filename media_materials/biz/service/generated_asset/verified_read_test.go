package generated_asset

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "media_agent/media_materials/biz/domain/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
)

type verifiedStorage struct {
	importStorage
	path, digest string
	calls        int
	invalid      bool
}

func (s *verifiedStorage) PresignVerifiedGet(_ context.Context, path, digest string) (objectstorage.ResolvedObject, error) {
	s.calls++
	s.path, s.digest = path, digest
	if s.invalid {
		return objectstorage.ResolvedObject{}, objectstorage.ErrConflict
	}
	return objectstorage.ResolvedObject{URL: "https://storage.example/verified", ExpiresAt: time.Now().Add(time.Minute).Unix()}, nil
}

func TestHistoricalReadUsesVerifiedSignerAndDoesNotFallbackOnFailure(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		storage := &verifiedStorage{importStorage: importStorage{err: errors.New("redundant storage read")}, invalid: invalid}
		s := NewService(&importRepo{}, importProduct{}, importVIP{})
		s.ConfigureHistoryImport(storage, importKinds{})
		a := &domain.GeneratedAsset{ObjectPath: "materials/org/ai_clothing/history.png", SourceSHA256: testDigest, Origin: "legacy_import"}
		err := s.resolveReadURL(t.Context(), a)
		if storage.calls != 1 || storage.path != a.ObjectPath || storage.digest != testDigest {
			t.Fatal("historical object verification bypassed")
		}
		if invalid {
			if err == nil || a.URL != "" {
				t.Fatal("verification failure returned a URL")
			}
		} else if err != nil || a.URL == "" {
			t.Fatal("verified read retried the object lookup")
		}
	}
}
