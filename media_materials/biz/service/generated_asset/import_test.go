package generated_asset

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	asset "media_agent/media_materials/biz/domain/generated_asset"
	product "media_agent/media_materials/biz/domain/product"
	vip "media_agent/media_materials/biz/domain/virtual_ip"
	"media_agent/media_materials/biz/shared/objectstorage"
)

const testDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type importRepo struct {
	asset *asset.GeneratedAsset
	saves int
}

func (r *importRepo) Save(_ context.Context, a *asset.GeneratedAsset) error {
	r.asset = a
	r.saves++
	return nil
}
func (r *importRepo) Get(_ context.Context, _ string) (*asset.GeneratedAsset, error) {
	return r.asset, nil
}
func (r *importRepo) GetByStepRun(context.Context, string) (*asset.GeneratedAsset, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *importRepo) GetByLegacySource(_ context.Context, _, _ string) (*asset.GeneratedAsset, error) {
	if r.asset == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.asset, nil
}
func (r *importRepo) List(context.Context, asset.ListQuery) ([]*asset.GeneratedAsset, int, error) {
	return nil, 0, nil
}
func (r *importRepo) ListBySource(context.Context, string, string) ([]*asset.GeneratedAsset, error) {
	return nil, nil
}
func (r *importRepo) Delete(context.Context, string) error { return nil }

type importProduct struct{}

func (importProduct) Get(_ context.Context, id string) (*product.Product, error) {
	return &product.Product{ID: id, OrganizationID: "org", Scope: "ai_clothing"}, nil
}

type importVIP struct{}

func (importVIP) Get(_ context.Context, id string) (*vip.VirtualIp, error) {
	return &vip.VirtualIp{ID: id, OrganizationID: "org", Scope: "ai_clothing"}, nil
}

type importStorage struct {
	info objectstorage.ObjectInfo
	err  error
}

func (s importStorage) Stat(context.Context, string) (objectstorage.ObjectInfo, error) {
	return s.info, s.err
}
func (s importStorage) PresignGet(context.Context, string) (objectstorage.ResolvedObject, error) {
	if s.err != nil {
		return objectstorage.ResolvedObject{}, s.err
	}
	return objectstorage.ResolvedObject{URL: "https://storage.example/imported.png", ExpiresAt: time.Now().Add(time.Minute).Unix()}, nil
}

type importKinds struct{}

func (importKinds) Lookup(context.Context, string) (string, string, string, error) {
	return "ASSET", "IMAGE", "ENABLED", nil
}

type videoKinds struct{}

func (videoKinds) Lookup(context.Context, string) (string, string, string, error) {
	return "ASSET", "VIDEO", "ENABLED", nil
}

func TestNativeOutputStoresVerifiedObjectPathNotSignedURL(t *testing.T) {
	repo := &importRepo{}
	svc := NewService(repo, importProduct{}, importVIP{})
	svc.ConfigureHistoryImport(importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "video/mp4"}}, videoKinds{})
	svc.ConfigureNativeOutputStorage("https://storage.example", "media")
	cmd := UpsertCommand{
		Scope: "ai_clothing", AssetKind: "ai_video", Name: "video-1", OwnerUserID: "user",
		URL:   "https://storage.example/media/materials/org/ai_clothing/generated_asset_import/result.mp4?signature=temporary",
		JobID: "job-1", StepRunID: "step-1", TemplateID: "tpl-1", TemplateVersion: 1, SourceProductID: "product",
	}
	asset, err := svc.Upsert(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if asset.ObjectPath != "materials/org/ai_clothing/generated_asset_import/result.mp4" || asset.URL != "" {
		t.Fatalf("native output did not freeze Object Path: %+v", asset)
	}
	for _, invalid := range []string{
		"https://evil.example/media/materials/org/ai_clothing/generated_asset_import/result.mp4",
		"https://storage.example/media/materials/other/ai_clothing/generated_asset_import/result.mp4",
		"https://storage.example/media/materials/org/ai_clothing/../other/result.mp4",
	} {
		repo = &importRepo{}
		svc = NewService(repo, importProduct{}, importVIP{})
		svc.ConfigureHistoryImport(importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "video/mp4"}}, videoKinds{})
		svc.ConfigureNativeOutputStorage("https://storage.example", "media")
		cmd.URL = invalid
		if _, err := svc.Upsert(context.Background(), cmd); err == nil || repo.saves != 0 {
			t.Fatalf("untrusted native output accepted: %q err=%v", invalid, err)
		}
	}
}

func TestGeneratedAssetReadSignsOnlyAuthorizedOrganizationMember(t *testing.T) {
	repo := &importRepo{asset: &asset.GeneratedAsset{
		ID: "gen-1", OrganizationID: "org", Scope: "ai_clothing", OwnerUserID: "owner",
		Visibility: asset.VisibilityPrivate, Origin: "legacy_import",
		ObjectPath: "materials/org/ai_clothing/generated_asset_import/frame.png", SourceSHA256: testDigest,
	}}
	svc := NewService(repo, importProduct{}, importVIP{})
	svc.ConfigureHistoryImport(importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "image/png"}}, importKinds{})
	if _, err := svc.GetForCaller(context.Background(), "gen-1", "other", nil, []string{"org"}); err == nil {
		t.Fatal("private asset was exposed to another member")
	}
	if _, err := svc.GetForCaller(context.Background(), "gen-1", "owner", nil, []string{"foreign"}); err == nil {
		t.Fatal("asset was exposed outside its organization")
	}
	got, err := svc.GetForCaller(context.Background(), "gen-1", "owner", nil, []string{"org"})
	if err != nil || got.URL == "" {
		t.Fatalf("owner could not read signed asset: asset=%+v err=%v", got, err)
	}
	repo.asset.Visibility = asset.VisibilityPublic
	if _, err := svc.GetForCaller(context.Background(), "gen-1", "other", nil, []string{"org"}); err != nil {
		t.Fatalf("same-organization public asset denied: %v", err)
	}
	if _, err := svc.Delete(context.Background(), "gen-1", "other", []string{"org"}); err == nil {
		t.Fatal("non-owner deleted generated asset")
	}
	if _, err := svc.Delete(context.Background(), "gen-1", "owner", []string{"org"}); err != nil {
		t.Fatalf("owner could not delete generated asset: %v", err)
	}
}

func TestImportHistoricalGeneratedAssetIsIdempotentAndHasNoFabricatedExecution(t *testing.T) {
	repo := &importRepo{}
	svc := NewService(repo, importProduct{}, importVIP{})
	svc.ConfigureHistoryImport(importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "image/png"}}, importKinds{})
	cmd := ImportCommand{
		OrganizationID: "org", Scope: "ai_clothing", AssetKind: "first_frame", Name: "legacy-frame-1",
		ObjectPath: "materials/org/ai_clothing/material/a.png", OwnerUserID: "user",
		SourceSystem: "media_ai", SourceRecordID: "frame-1", SourceSHA256: testDigest,
		SourceCreatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), SourceProductID: "product", SourceVirtualIPID: "ip",
	}
	first, err := svc.Import(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Import(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if repo.saves != 1 || first.ID != second.ID || first.Origin != "legacy_import" || first.JobID != "" || first.StepRunID != "" || first.TemplateID != "" || first.SourceCreatedAt == nil {
		t.Fatalf("invalid imported asset or duplicate save: first=%+v saves=%d", first, repo.saves)
	}
	resolvedAsset, resolved, err := svc.ResolveImported(context.Background(), first.ID)
	if err != nil || resolvedAsset.ID != first.ID || resolved.URL == "" {
		t.Fatalf("imported asset cannot be resolved: asset=%+v resolved=%+v err=%v", resolvedAsset, resolved, err)
	}
	cmd.SourceSHA256 = strings.Repeat("b", 64)
	if _, err := svc.Import(context.Background(), cmd); err == nil {
		t.Fatal("changed digest accepted")
	}
}

func TestImportRejectsUncontrolledObjectAndSourceConflict(t *testing.T) {
	repo := &importRepo{}
	svc := NewService(repo, importProduct{}, importVIP{})
	svc.ConfigureHistoryImport(importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "image/png"}}, importKinds{})
	cmd := ImportCommand{OrganizationID: "org", Scope: "ai_clothing", AssetKind: "first_frame", Name: "frame",
		ObjectPath: "https://outside.example/a.png", OwnerUserID: "user", SourceSystem: "media_ai", SourceRecordID: "frame",
		SourceSHA256: testDigest, SourceCreatedAt: time.Now(), SourceProductID: "product"}
	if _, err := svc.Import(context.Background(), cmd); err == nil {
		t.Fatal("external URL accepted")
	}
	cmd.ObjectPath = "materials/org/ai_clothing/material/a.png"
	if _, err := svc.Import(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	cmd.Name = "changed"
	if _, err := svc.Import(context.Background(), cmd); !errors.Is(err, ErrImportConflict) {
		t.Fatalf("source conflict: %v", err)
	}
}

func TestImportRejectsMissingMappingCrossOrganizationAndUnreachableObject(t *testing.T) {
	base := ImportCommand{
		OrganizationID: "org", Scope: "ai_clothing", AssetKind: "first_frame", Name: "frame",
		ObjectPath: "materials/org/ai_clothing/generated_asset_import/a.png", OwnerUserID: "user",
		SourceSystem: "media_ai", SourceRecordID: "frame", SourceSHA256: testDigest,
		SourceCreatedAt: time.Now(), SourceProductID: "product", SourceVirtualIPID: "ip",
	}
	for _, tc := range []struct {
		name   string
		change func(*ImportCommand)
		store  importStorage
	}{
		{"cross organization", func(c *ImportCommand) { c.OrganizationID = "another" }, importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "image/png"}}},
		{"missing mapping", func(c *ImportCommand) { c.SourceProductID, c.SourceVirtualIPID = "", "" }, importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "image/png"}}},
		{"unreachable object", func(*ImportCommand) {}, importStorage{err: errors.New("storage unavailable")}},
		{"wrong object digest", func(*ImportCommand) {}, importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: strings.Repeat("b", 64), ContentType: "image/png"}}},
		{"wrong object media type", func(*ImportCommand) {}, importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "video/mp4"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &importRepo{}
			svc := NewService(repo, importProduct{}, importVIP{})
			svc.ConfigureHistoryImport(tc.store, importKinds{})
			cmd := base
			tc.change(&cmd)
			if _, err := svc.Import(context.Background(), cmd); err == nil {
				t.Fatal("unsafe import accepted")
			}
			if repo.saves != 0 {
				t.Fatalf("rejected import wrote %d assets", repo.saves)
			}
		})
	}
}

func TestNativeOutputUsesTrustedJobOrganizationWithoutSources(t *testing.T) {
	for _, tc := range []struct {
		name, org, product, vip string
		wantErr                 bool
	}{
		{"generic job", "org", "", "", false},
		{"matching sources", "org", "product", "vip", false},
		{"legacy source", "", "product", "", false},
		{"missing tenant", "", "", "", true},
		{"foreign product", "other", "product", "", true},
		{"foreign vip", "other", "", "vip", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &importRepo{}
			svc := NewService(repo, importProduct{}, importVIP{})
			svc.ConfigureHistoryImport(importStorage{info: objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "video/mp4"}}, videoKinds{})
			svc.ConfigureNativeOutputStorage("https://storage.example", "media")
			got, err := svc.Upsert(context.Background(), UpsertCommand{OrganizationID: tc.org, Scope: "ai_clothing", AssetKind: "ai_video", OwnerUserID: "user", JobID: "job", StepRunID: "step", TemplateID: "template", SourceProductID: tc.product, SourceVirtualIPID: tc.vip, URL: "https://storage.example/media/materials/org/ai_clothing/worker_generated/result.mp4"})
			if tc.wantErr {
				if err == nil || repo.saves != 0 {
					t.Fatal("untrusted tenant accepted", err)
				}
			} else if err != nil || got.OrganizationID != "org" || got.ObjectPath == "" || repo.saves != 1 {
				t.Fatal("trusted tenant output not registered", err)
			}
		})
	}
}
