package generated_asset

import (
	"context"
	"errors"
	"strings"

	domain "media_agent/media_materials/biz/domain/generated_asset"
)

// ConfigureResultReceipts wires the receipt repository and caller transaction.
func (s *Service) ConfigureResultReceipts(results domain.ResultRepo, tx interface {
	RunInTx(context.Context, func(context.Context) error) error
}) {
	s.results, s.resultTx = results, tx
}

func (s *Service) verifyResultObject(ctx context.Context, a *domain.GeneratedAsset) error {
	if s.assetKinds == nil {
		return errors.New("output asset kind lookup is not configured")
	}
	kind, mediaType, status, err := s.assetKinds.Lookup(ctx, a.AssetKind)
	if err != nil || kind != "ASSET" || status != "ENABLED" && status != "SYSTEM" {
		return errors.New("output asset kind is not enabled")
	}
	if a.ObjectPath == "" {
		if mediaType != "TEXT" || strings.TrimSpace(a.Content) == "" || a.URL != "" {
			return errors.New("output does not match text media type")
		}
		return nil
	}
	if s.historyStorage == nil {
		return errors.New("native output storage is not configured")
	}
	info, err := s.historyStorage.Stat(ctx, a.ObjectPath)
	if err != nil || info.SizeBytes <= 0 || !sha256Pattern.MatchString(info.Checksum) ||
		mediaType == "IMAGE" && !strings.HasPrefix(info.ContentType, "image/") ||
		mediaType == "VIDEO" && !strings.HasPrefix(info.ContentType, "video/") ||
		mediaType != "IMAGE" && mediaType != "VIDEO" {
		return errors.New("native output object is unavailable or has the wrong media type")
	}
	return nil
}

func (s *Service) replayResult(ctx context.Context, receipt *domain.ResultReceipt, digest string) (*domain.GeneratedAsset, error) {
	if receipt.RequestDigest != digest {
		return nil, domain.ErrResultConflict
	}
	accepted, err := s.repo.Get(ctx, receipt.AssetID)
	if err != nil || accepted == nil {
		// Receipt is retained after soft deletion. Never recreate this generation.
		return nil, domain.ErrResultConflict
	}
	acceptedDigest, err := domain.ResultDigest(accepted)
	if err != nil || acceptedDigest != digest {
		return nil, domain.ErrResultConflict
	}
	return accepted, nil
}

func (s *Service) registerResult(ctx context.Context, key string, a *domain.GeneratedAsset) (*domain.GeneratedAsset, error) {
	// Full generation identity avoids the existing organization/scope/kind/name
	// uniqueness constraint colliding across different successful rounds.
	a.Name += "-" + key
	digest, err := domain.ResultDigest(a)
	if err != nil {
		return nil, err
	}
	receipt, err := s.results.GetResult(ctx, a.StepRunID, key, false)
	if err != nil {
		return nil, err
	}
	if receipt != nil {
		return s.replayResult(ctx, receipt, digest)
	}
	// No remote storage call runs while holding the result transaction lock.
	if err := s.verifyResultObject(ctx, a); err != nil {
		return nil, err
	}
	var accepted *domain.GeneratedAsset
	err = s.resultTx.RunInTx(ctx, func(txCtx context.Context) error {
		receipt, err := s.results.GetResult(txCtx, a.StepRunID, key, true)
		if err != nil {
			return err
		}
		if receipt != nil {
			accepted, err = s.replayResult(txCtx, receipt, digest)
			return err
		}
		if err := s.repo.Save(txCtx, a); err != nil {
			return err
		}
		if err := s.results.SaveResult(txCtx, domain.ResultReceipt{
			StepRunID: a.StepRunID, ResultKey: key, AssetID: a.ID, RequestDigest: digest,
		}); err != nil {
			return err
		}
		accepted = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return accepted, nil
}
