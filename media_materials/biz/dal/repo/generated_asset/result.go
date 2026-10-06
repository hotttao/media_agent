package generated_asset

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"media_agent/media_materials/biz/dal"
	domain "media_agent/media_materials/biz/domain/generated_asset"
)

func (r *Repo) GetResult(ctx context.Context, stepRunID, resultKey string, lock bool) (*domain.ResultReceipt, error) {
	if err := domain.ValidateResultIdentity(stepRunID, resultKey); err != nil {
		return nil, err
	}
	db := dal.FromContext(ctx, r.db).WithContext(ctx)
	if lock {
		if dal.FromContext(ctx, r.db) == r.db {
			return nil, errors.New("generation result transaction required")
		}
		encoded, _ := json.Marshal([]string{stepRunID, resultKey})
		hash := sha256.Sum256(encoded)
		if err := db.Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(hash[:8]))).Error; err != nil {
			return nil, err
		}
	}
	var receipt domain.ResultReceipt
	err := db.Table("media_materials.generated_asset_results").Select("step_run_id,result_key,asset_id,request_digest").Where("step_run_id=? AND result_key=?", stepRunID, resultKey).Take(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (r *Repo) SaveResult(ctx context.Context, receipt domain.ResultReceipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	tx := dal.FromContext(ctx, r.db)
	if tx == r.db {
		return errors.New("generation result transaction required")
	}
	result := tx.WithContext(ctx).Exec(`INSERT INTO media_materials.generated_asset_results
        (step_run_id,result_key,asset_id,request_digest)
        SELECT ?,?,?,? WHERE EXISTS (SELECT 1 FROM media_materials.generated_assets
        WHERE id=? AND step_run_id=? AND origin='native' AND deleted_at IS NULL)
        ON CONFLICT DO NOTHING`, receipt.StepRunID, receipt.ResultKey, receipt.AssetID, receipt.RequestDigest, receipt.AssetID, receipt.StepRunID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrResultConflict
	}
	return nil
}
