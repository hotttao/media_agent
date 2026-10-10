package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrCapabilityNotFound
		}
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
	for _, input := range inputs {
		if !input.ValidMediaConstraint() {
			return nil, fmt.Errorf("invalid capability input media constraint")
		}
	}
	var outputMedia domain.MediaType
	if row.OutputMediaType != nil {
		outputMedia = domain.MediaType(*row.OutputMediaType)
		if !outputMedia.Valid() || row.OutputAssetKind != "" {
			return nil, fmt.Errorf("invalid provider capability output contract")
		}
	}
	var retry struct {
		RetryCount        int    `json:"retry_count"`
		RetryLogic        string `json:"retry_logic"`
		RetryDelaySeconds int    `json:"retry_delay_seconds"`
	}
	if err := json.Unmarshal(row.RetryPolicy, &retry); err != nil {
		return nil, err
	}
	executionTarget := domain.ExecutionTarget(row.ExecutionTarget)
	if !executionTarget.Valid() {
		return nil, fmt.Errorf("invalid execution target %q for capability %s@%d", row.ExecutionTarget, row.CapabilityKey, row.Version)
	}
	managedPoolKey := ""
	if row.ManagedPoolKey != nil {
		managedPoolKey = *row.ManagedPoolKey
	}
	if executionTarget == domain.ExecutionTargetPersonalWorker && managedPoolKey != "" {
		return nil, fmt.Errorf("personal capability %s@%d cannot declare managed pool", row.CapabilityKey, row.Version)
	}
	if executionTarget == domain.ExecutionTargetManagedWorker && managedPoolKey == "" {
		return nil, fmt.Errorf("managed capability %s@%d requires managed pool", row.CapabilityKey, row.Version)
	}
	result := &domain.Capability{
		Key: row.CapabilityKey, Version: row.Version, Provider: row.Provider, Mode: row.Mode,
		Inputs: inputs, OutputAssetKind: row.OutputAssetKind, OutputMediaType: outputMedia, ConductorTaskDef: row.ConductorTaskDef,
		ExecutionTarget: executionTarget, ManagedPoolKey: managedPoolKey,
		RetryCount: retry.RetryCount, RetryLogic: retry.RetryLogic,
		RetryDelaySeconds: retry.RetryDelaySeconds, TimeoutSeconds: row.TimeoutSeconds,
		Status: row.Status, CreatedAt: row.CreatedAt, PublishedAt: row.PublishedAt,
	}
	if row.GenerationMethod != nil {
		result.GenerationMethod = domain.GenerationMethod(*row.GenerationMethod)
	}
	if row.GenerationType != nil {
		result.GenerationType = domain.GenerationType(*row.GenerationType)
	}
	if err := result.ValidateContract(); err != nil {
		return nil, err
	}
	return result, nil
}
