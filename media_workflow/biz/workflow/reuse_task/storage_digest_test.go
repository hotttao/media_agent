package reuse_task

import (
	"context"
	"errors"
	capability "media_agent/media_workflow/biz/domain/capability"
	"strings"
	"testing"

	"media_agent/media_workflow/biz/shared/client"
)

type objectLookup struct {
	refs   []string
	result client.ResolvedStorageObject
	err    error
}

func (s *objectLookup) ResolveInternalObject(_ context.Context, org, ref string) (client.ResolvedStorageObject, error) {
	s.refs = append(s.refs, org+":"+ref)
	return s.result, s.err
}
func TestStorageDigestUsesOnlyOrganizationScopedTrustedReferences(t *testing.T) {
	objects := &objectLookup{result: client.ResolvedStorageObject{ChecksumSHA256: strings.Repeat("a", 64), SizeBytes: 100, ContentType: "image/png"}}
	resolver := StorageDigests{Objects: objects, StorageOrigin: "https://storage.test"}
	for _, ref := range []string{"object_path:materials/org/image.png", "materials/org/image.png", "https://storage.test/media/materials/org/image.png?signature=temporary"} {
		got, err := resolver.Checksum(context.Background(), "org", ref, capability.MediaImage)
		if err != nil || got != strings.Repeat("a", 64) {
			t.Fatal("valid storage reference did not resolve", err)
		}
	}
	before := len(objects.refs)
	for _, ref := range []string{"https://provider.test/image.png", "https://user@storage.test/media/materials/org/image.png", "https://storage.test/media/materials/org/image.png#fragment", "materials/other/image.png", "object_path:materials/org/../other/image.png", "object_path:materials/org/a\\b", "gen_unknown"} {
		if _, err := resolver.Checksum(context.Background(), "org", ref, capability.MediaImage); !errors.Is(err, ErrInvalidFingerprintInput) {
			t.Fatal("untrusted media reached storage", err)
		}
	}
	if len(objects.refs) != before {
		t.Fatal("untrusted input performed a storage lookup")
	}
	objects.err = errors.New("storage unavailable")
	if _, err := resolver.Checksum(context.Background(), "org", "materials/org/image.png", capability.MediaImage); !errors.Is(err, objects.err) {
		t.Fatal("storage outage was masked", err)
	}
}
