package capability

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/capability"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Get(ctx context.Context, key string, version int) (*domain.Capability, error) {
	var row model.ExecutionCapability
	err := dal.FromContext(ctx, r.db).WithContext(ctx).
		Where("capability_key = ? AND version = ? AND status = ?", key, version, "published").
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return toDomain(&row)
}

func (r *Repo) ListPublished(ctx context.Context) ([]*domain.Capability, error) {
	var rows []model.ExecutionCapability
	if err := dal.FromContext(ctx, r.db).WithContext(ctx).
		Where("status = ?", "published").Order("capability_key, version").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]*domain.Capability, 0, len(rows))
	for i := range rows {
		item, err := toDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func toDomain(row *model.ExecutionCapability) (*domain.Capability, error) {
	var inputs []domain.Input
	if err := json.Unmarshal(row.InputSchema, &inputs); err != nil {
		return nil, err
	}
	var retry struct {
		RetryCount        int    `json:"retry_count"`
		RetryLogic        string `json:"retry_logic"`
		RetryDelaySeconds int    `json:"retry_delay_seconds"`
	}
	if err := json.Unmarshal(row.RetryPolicy, &retry); err != nil {
		return nil, err
	}
	return &domain.Capability{
		Key: row.CapabilityKey, Version: row.Version, Provider: row.Provider, Mode: row.Mode,
		Inputs: inputs, OutputAssetKind: row.OutputAssetKind, ConductorTaskDef: row.ConductorTaskDef,
		RetryCount: retry.RetryCount, RetryLogic: retry.RetryLogic,
		RetryDelaySeconds: retry.RetryDelaySeconds, TimeoutSeconds: row.TimeoutSeconds,
		Status: row.Status, CreatedAt: row.CreatedAt, PublishedAt: row.PublishedAt,
	}, nil
}
