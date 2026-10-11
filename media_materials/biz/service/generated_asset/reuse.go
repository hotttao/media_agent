package generated_asset

import (
	"context"
	"errors"
	"path"
	"strings"

	"gorm.io/gorm"
	domain "media_agent/media_materials/biz/domain/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
)

var (
	ErrReusableAssetNotFound        = errors.New("reusable generated asset not found")
	ErrReuseVerificationUnavailable = errors.New("reusable asset verification unavailable")
)

func (s *Service) VerifyReusable(ctx context.Context, id, organizationID, scope, ownerUserID string) (*domain.GeneratedAsset, error) {
	if id == "" || organizationID == "" || scope == "" || ownerUserID == "" || strings.ContainsAny(organizationID+scope, "/\\") {
		return nil, ErrReusableAssetNotFound
	}
	a, err := s.repo.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReusableAssetNotFound
	}
	if err != nil {
		return nil, ErrReuseVerificationUnavailable
	}
	if a == nil || a.ID != id || a.Origin != "native" || a.OrganizationID != organizationID || a.Scope != scope || a.OwnerUserID != ownerUserID || a.JobID == "" || a.StepRunID == "" || a.TemplateID == "" || a.TemplateVersion <= 0 || a.AssetKind == "" || !canReadGeneratedAsset(a, ownerUserID, nil, []string{organizationID}) {
		return nil, ErrReusableAssetNotFound
	}
	if a.ObjectPath == "" {
		if strings.TrimSpace(a.Content) == "" {
			return nil, ErrReusableAssetNotFound
		}
		return a, nil
	}
	if !strings.HasPrefix(a.ObjectPath, "materials/"+organizationID+"/"+scope+"/") || path.Clean(a.ObjectPath) != a.ObjectPath || strings.ContainsAny(a.ObjectPath, "\\?#") {
		return nil, ErrReusableAssetNotFound
	}
	if s.historyStorage == nil {
		return nil, ErrReuseVerificationUnavailable
	}
	info, err := s.historyStorage.Stat(ctx, a.ObjectPath)
	if errors.Is(err, objectstorage.ErrNotFound) {
		return nil, ErrReusableAssetNotFound
	}
	if err != nil {
		return nil, ErrReuseVerificationUnavailable
	}
	if info.SizeBytes <= 0 {
		return nil, ErrReusableAssetNotFound
	}
	return a, nil
}
