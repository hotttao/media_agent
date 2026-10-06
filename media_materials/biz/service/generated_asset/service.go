package generated_asset

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	domain "media_agent/media_materials/biz/domain/generated_asset"
	productdomain "media_agent/media_materials/biz/domain/product"
	vipdomain "media_agent/media_materials/biz/domain/virtual_ip"
	"media_agent/media_materials/biz/shared"
	"media_agent/media_materials/biz/shared/objectstorage"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var ErrImportConflict = errors.New("historical generated asset source conflict")

// Service generated_asset 用例编排。产物由 workflow 落（Upsert）；前端只读 + 删。
type Service struct {
	repo     domain.Repo
	results  domain.ResultRepo
	resultTx interface {
		RunInTx(context.Context, func(context.Context) error) error
	}
	nativeOutputOrigin string
	nativeOutputBucket string
	historyStorage     interface {
		Stat(context.Context, string) (objectstorage.ObjectInfo, error)
		PresignGet(context.Context, string) (objectstorage.ResolvedObject, error)
	}
	assetKinds interface {
		Lookup(context.Context, string) (string, string, string, error)
	}
	products interface {
		Get(context.Context, string) (*productdomain.Product, error)
	}
	virtualIPs interface {
		Get(context.Context, string) (*vipdomain.VirtualIp, error)
	}
}

// ConfigureNativeOutputStorage makes native binary outputs durable: a Worker
// candidate URL must point at a pre-uploaded object in our own bucket. The URL
// itself is never persisted as the Generated Asset's storage identity.
func (s *Service) ConfigureNativeOutputStorage(publicOrigin, bucket string) {
	s.nativeOutputOrigin = strings.TrimRight(publicOrigin, "/")
	s.nativeOutputBucket = bucket
}

// ConfigureHistoryImport wires trusted object verification and Asset Kind
// lookup before serving requests. Normal generated-asset upsert is unchanged.
func (s *Service) ConfigureHistoryImport(storage interface {
	Stat(context.Context, string) (objectstorage.ObjectInfo, error)
	PresignGet(context.Context, string) (objectstorage.ResolvedObject, error)
}, kinds interface {
	Lookup(context.Context, string) (string, string, string, error)
}) {
	s.historyStorage = storage
	s.assetKinds = kinds
}

// ResolveImported returns a short-lived URL only after revalidating the
// immutable imported object against its recorded digest.
func (s *Service) ResolveImported(ctx context.Context, id string) (*domain.GeneratedAsset, objectstorage.ResolvedObject, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil || a.Origin != "legacy_import" || a.SourceCreatedAt == nil || s.historyStorage == nil {
		return nil, objectstorage.ResolvedObject{}, gorm.ErrRecordNotFound
	}
	info, err := s.historyStorage.Stat(ctx, a.ObjectPath)
	if err != nil || info.SizeBytes <= 0 || info.Checksum != a.SourceSHA256 {
		return nil, objectstorage.ResolvedObject{}, errors.New("historical object is unavailable or its digest changed")
	}
	resolved, err := s.historyStorage.PresignGet(ctx, a.ObjectPath)
	if err != nil || resolved.URL == "" {
		return nil, objectstorage.ResolvedObject{}, errors.New("cannot resolve historical object")
	}
	return a, resolved, nil
}

func NewService(repo domain.Repo, products interface {
	Get(context.Context, string) (*productdomain.Product, error)
}, virtualIPs interface {
	Get(context.Context, string) (*vipdomain.VirtualIp, error)
}) *Service {
	return &Service{repo: repo, products: products, virtualIPs: virtualIPs}
}

// Upsert 由 media_workflow 调用：落产物 + 生成记录字段。
// asset_kind 校验（resource_type=ASSET）由 workflow 侧 BatchGetAssetKind 已做；此处只兜底非空。
func (s *Service) Upsert(ctx context.Context, cmd UpsertCommand) (*domain.GeneratedAsset, error) {
	if cmd.ResultKey != "" {
		if err := domain.ValidateResultIdentity(cmd.StepRunID, cmd.ResultKey); err != nil {
			return nil, err
		}
		if s.results == nil || s.resultTx == nil {
			return nil, errors.New("generation result persistence is not configured")
		}
	}
	if strings.TrimSpace(cmd.AssetKind) == "" {
		return nil, errors.New("asset_kind is required")
	}
	if strings.TrimSpace(cmd.URL) == "" && strings.TrimSpace(cmd.Content) == "" {
		return nil, errors.New("url or content is required")
	}
	if cmd.JobID == "" || cmd.StepRunID == "" || cmd.TemplateID == "" {
		return nil, errors.New("job_id / step_run_id / template_id are required")
	}
	if cmd.ResultKey == "" {
		if existing, err := s.repo.GetByStepRun(ctx, cmd.StepRunID); err == nil {
			return existing, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("lookup existing generated_asset: %w", err)
		}
	}
	var organizationID string
	if cmd.SourceProductID != "" {
		product, err := s.products.Get(ctx, cmd.SourceProductID)
		if err != nil {
			return nil, fmt.Errorf("resolve source product: %w", err)
		}
		if product.Scope != cmd.Scope {
			return nil, errors.New("source product scope does not match output")
		}
		organizationID = product.OrganizationID
	}
	if cmd.SourceVirtualIPID != "" {
		vip, err := s.virtualIPs.Get(ctx, cmd.SourceVirtualIPID)
		if err != nil {
			return nil, fmt.Errorf("resolve source virtual IP: %w", err)
		}
		if vip.Scope != cmd.Scope {
			return nil, errors.New("source virtual IP scope does not match output")
		}
		if organizationID != "" && organizationID != vip.OrganizationID {
			return nil, errors.New("source product and virtual IP belong to different organizations")
		}
		organizationID = vip.OrganizationID
	}
	if organizationID == "" {
		return nil, errors.New("source product or virtual IP is required to resolve organization")
	}
	objectPath := ""
	if cmd.URL != "" && s.nativeOutputOrigin != "" {
		var err error
		objectPath, err = nativeObjectPath(cmd.URL, s.nativeOutputOrigin, s.nativeOutputBucket, organizationID, cmd.Scope)
		if err != nil {
			return nil, err
		}
		if s.historyStorage == nil || s.assetKinds == nil {
			return nil, errors.New("native output storage is not configured")
		}
	}
	vis := domain.Visibility(cmd.Visibility)
	if vis == "" {
		vis = domain.VisibilityPrivate
	}
	a := &domain.GeneratedAsset{
		ID:             "gen_" + uuid.NewString(),
		Origin:         "native",
		OrganizationID: organizationID,
		Scope:          cmd.Scope, Name: cmd.Name, AssetKind: cmd.AssetKind,
		URL: cmd.URL, ObjectPath: objectPath, Content: cmd.Content, OwnerUserID: cmd.OwnerUserID,
		Visibility: vis, TeamID: cmd.TeamID,
		JobID: cmd.JobID, StepRunID: cmd.StepRunID, TemplateID: cmd.TemplateID,
		TemplateVersion:   cmd.TemplateVersion,
		SourceVirtualIPID: cmd.SourceVirtualIPID, SourceProductID: cmd.SourceProductID,
	}
	if objectPath != "" {
		a.URL = ""
	}
	if cmd.ResultKey != "" {
		return s.registerResult(ctx, cmd.ResultKey, a)
	}
	if objectPath != "" {
		if err := s.verifyResultObject(ctx, a); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Save(ctx, a); err != nil {
		return nil, fmt.Errorf("save generated_asset: %w", err)
	}
	return s.repo.Get(ctx, a.ID)
}

func nativeObjectPath(raw, publicOrigin, bucket, organizationID, scope string) (string, error) {
	objectURL, err := url.Parse(raw)
	if err != nil || objectURL.User != nil || objectURL.Fragment != "" || objectURL.Host == "" {
		return "", errors.New("invalid native output URL")
	}
	origin, err := url.Parse(publicOrigin)
	if err != nil || origin.Scheme == "" || origin.Host == "" || objectURL.Scheme != origin.Scheme || objectURL.Host != origin.Host ||
		bucket == "" || organizationID == "" || scope == "" || strings.ContainsAny(organizationID, "/\\") || strings.ContainsAny(scope, "/\\") {
		return "", errors.New("native output is not in controlled storage")
	}
	prefix := "/" + bucket + "/materials/" + organizationID + "/" + scope + "/"
	if !strings.HasPrefix(objectURL.Path, prefix) || path.Clean(objectURL.Path) != objectURL.Path || strings.Contains(objectURL.Path, "\\") {
		return "", errors.New("native output is outside organization/scope storage namespace")
	}
	return strings.TrimPrefix(objectURL.Path, "/"+bucket+"/"), nil
}

// Import records one historical output without fabricating a Job, StepRun, or
// Template. The target object must already be inside the organization's
// storage namespace and have the exact source digest.
func (s *Service) Import(ctx context.Context, cmd ImportCommand) (*domain.GeneratedAsset, error) {
	if s.historyStorage == nil || s.assetKinds == nil {
		return nil, errors.New("history import dependencies are not configured")
	}
	if cmd.SourceSystem != "media_ai" || cmd.SourceRecordID == "" || len(cmd.SourceRecordID) > 128 ||
		cmd.OrganizationID == "" || !shared.IsKnownScope(cmd.Scope) || cmd.OwnerUserID == "" || cmd.Name == "" ||
		cmd.SourceCreatedAt.IsZero() || !sha256Pattern.MatchString(cmd.SourceSHA256) ||
		cmd.SourceProductID == "" && cmd.SourceVirtualIPID == "" {
		return nil, errors.New("incomplete historical generated asset metadata")
	}
	prefix := "materials/" + cmd.OrganizationID + "/" + cmd.Scope + "/"
	if !strings.HasPrefix(cmd.ObjectPath, prefix) || path.Clean(cmd.ObjectPath) != cmd.ObjectPath {
		return nil, errors.New("historical object is outside organization/scope namespace")
	}
	if cmd.SourceProductID != "" {
		product, err := s.products.Get(ctx, cmd.SourceProductID)
		if err != nil || product.OrganizationID != cmd.OrganizationID || product.Scope != cmd.Scope {
			return nil, errors.New("source product is not in target organization/scope")
		}
	}
	if cmd.SourceVirtualIPID != "" {
		vip, err := s.virtualIPs.Get(ctx, cmd.SourceVirtualIPID)
		if err != nil || vip.OrganizationID != cmd.OrganizationID || vip.Scope != cmd.Scope {
			return nil, errors.New("source virtual IP is not in target organization/scope")
		}
	}
	resourceType, mediaType, status, err := s.assetKinds.Lookup(ctx, cmd.AssetKind)
	if err != nil || resourceType != "ASSET" || status != "ENABLED" && status != "SYSTEM" {
		return nil, errors.New("asset_kind is not an enabled generated asset kind")
	}
	info, err := s.historyStorage.Stat(ctx, cmd.ObjectPath)
	if err != nil {
		return nil, fmt.Errorf("verify historical object: %w", err)
	}
	if info.SizeBytes <= 0 || info.Checksum != cmd.SourceSHA256 ||
		mediaType == "IMAGE" && !strings.HasPrefix(info.ContentType, "image/") ||
		mediaType == "VIDEO" && !strings.HasPrefix(info.ContentType, "video/") ||
		mediaType != "IMAGE" && mediaType != "VIDEO" {
		return nil, errors.New("historical object digest or media type does not match")
	}
	if existing, err := s.repo.GetByLegacySource(ctx, cmd.SourceSystem, cmd.SourceRecordID); err == nil {
		return matchImported(existing, cmd)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup historical generated asset: %w", err)
	}
	now := time.Now().UTC()
	created := cmd.SourceCreatedAt.UTC()
	a := &domain.GeneratedAsset{
		ID: "gen_" + uuid.NewString(), OrganizationID: cmd.OrganizationID, Scope: cmd.Scope,
		Name: cmd.Name, AssetKind: cmd.AssetKind, ObjectPath: cmd.ObjectPath,
		OwnerUserID: cmd.OwnerUserID, Visibility: domain.VisibilityPrivate,
		Origin: "legacy_import", SourceSystem: cmd.SourceSystem, SourceRecordID: cmd.SourceRecordID,
		SourceSHA256: cmd.SourceSHA256, SourceCreatedAt: &created, ImportedAt: &now,
		SourceVirtualIPID: cmd.SourceVirtualIPID, SourceProductID: cmd.SourceProductID,
	}
	if err := s.repo.Save(ctx, a); err != nil {
		// Unique source key resolves concurrent same-content imports without
		// suppressing an unrelated storage or database failure.
		if existing, lookupErr := s.repo.GetByLegacySource(ctx, cmd.SourceSystem, cmd.SourceRecordID); lookupErr == nil {
			return matchImported(existing, cmd)
		}
		return nil, fmt.Errorf("save historical generated asset: %w", err)
	}
	return s.repo.Get(ctx, a.ID)
}

func matchImported(existing *domain.GeneratedAsset, cmd ImportCommand) (*domain.GeneratedAsset, error) {
	if existing.Origin != "legacy_import" || existing.OrganizationID != cmd.OrganizationID || existing.Scope != cmd.Scope ||
		existing.OwnerUserID != cmd.OwnerUserID || existing.AssetKind != cmd.AssetKind || existing.ObjectPath != cmd.ObjectPath ||
		existing.SourceSHA256 != cmd.SourceSHA256 || existing.SourceProductID != cmd.SourceProductID ||
		existing.SourceVirtualIPID != cmd.SourceVirtualIPID || existing.Name != cmd.Name ||
		existing.SourceCreatedAt == nil || !existing.SourceCreatedAt.Equal(cmd.SourceCreatedAt) {
		return nil, ErrImportConflict
	}
	return existing, nil
}

func (s *Service) Get(ctx context.Context, id string) (*domain.GeneratedAsset, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("generated_asset %q: %w", id, err)
	}
	return a, nil
}

func (s *Service) GetForCaller(ctx context.Context, id, callerID string, callerTeams, organizationIDs []string) (*domain.GeneratedAsset, error) {
	a, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canReadGeneratedAsset(a, callerID, callerTeams, organizationIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	if err := s.resolveReadURL(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) List(ctx context.Context, q ListQuery) ([]*domain.GeneratedAsset, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	for _, item := range items {
		if err := s.resolveReadURL(ctx, item); err != nil {
			return nil, 0, err
		}
	}
	return items, total, nil
}

func canReadGeneratedAsset(a *domain.GeneratedAsset, callerID string, callerTeams, organizationIDs []string) bool {
	if a == nil || callerID == "" {
		return false
	}
	member := false
	for _, id := range organizationIDs {
		if id == a.OrganizationID {
			member = true
			break
		}
	}
	if !member {
		return false
	}
	if a.OwnerUserID == callerID || a.Visibility == domain.VisibilityPublic {
		return true
	}
	if a.Visibility == domain.VisibilityTeam && a.TeamID != "" {
		for _, team := range callerTeams {
			if team == a.TeamID {
				return true
			}
		}
	}
	return false
}

func (s *Service) resolveReadURL(ctx context.Context, a *domain.GeneratedAsset) error {
	if a.ObjectPath == "" {
		return nil
	}
	if s.historyStorage == nil {
		return errors.New("generated asset storage is unavailable")
	}
	if a.Origin == "legacy_import" {
		info, err := s.historyStorage.Stat(ctx, a.ObjectPath)
		if err != nil || info.Checksum != a.SourceSHA256 {
			return errors.New("historical generated asset digest no longer matches")
		}
	}
	resolved, err := s.historyStorage.PresignGet(ctx, a.ObjectPath)
	if err != nil || resolved.URL == "" || resolved.ExpiresAt <= time.Now().Unix() {
		return errors.New("generated asset storage object is unavailable")
	}
	a.URL = resolved.URL
	return nil
}

func (s *Service) ListBySource(ctx context.Context, sourceKind, sourceID string) ([]*domain.GeneratedAsset, error) {
	if sourceKind != "virtual_ip" && sourceKind != "product" {
		return nil, fmt.Errorf("invalid source_kind %q", sourceKind)
	}
	return s.repo.ListBySource(ctx, sourceKind, sourceID)
}

func (s *Service) Delete(ctx context.Context, id, callerID string, organizationIDs []string) (*domain.GeneratedAsset, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if callerID == "" || a.OwnerUserID != callerID || !canReadGeneratedAsset(a, callerID, nil, organizationIDs) {
		return nil, gorm.ErrRecordNotFound
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete generated_asset: %w", err)
	}
	return a, nil
}
