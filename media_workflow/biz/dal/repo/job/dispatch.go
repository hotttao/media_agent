package job

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	domain "media_agent/media_workflow/biz/domain/job"
)

type DispatchRepo struct{ db *gorm.DB }

func NewDispatchRepo(db *gorm.DB) *DispatchRepo { return &DispatchRepo{db: db} }

func (r *DispatchRepo) conn(ctx context.Context) *gorm.DB {
	return dal.FromContext(ctx, r.db).WithContext(ctx)
}

func (r *DispatchRepo) FindActive(ctx context.Context, userID, organizationID, credentialID, workerInstanceID, capabilityKey string, capabilityVersion int, now time.Time) (*domain.WorkerDispatch, error) {
	var row model.WorkerDispatch
	err := r.conn(ctx).
		Where("user_id = ? AND organization_id = ? AND credential_id = ? AND worker_instance_id = ? AND capability_key = ? AND capability_version = ? AND status IN ? AND lease_expires_at > ?",
			userID, organizationID, credentialID, workerInstanceID, capabilityKey, capabilityVersion,
			[]string{string(domain.DispatchClaimed), string(domain.DispatchDelivered), string(domain.DispatchRunning)}, now).
		Order("created_at").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrDispatchNotFound
	}
	if err != nil {
		return nil, err
	}
	return dispatchToDomain(&row), nil
}

func (r *DispatchRepo) Create(ctx context.Context, dispatch *domain.WorkerDispatch) (*domain.WorkerDispatch, error) {
	deliveredAt := dispatch.DeliveredAt
	row := model.WorkerDispatch{
		ID: dispatch.ID, ConductorTaskID: dispatch.ConductorTaskID, ConductorWorkflowID: dispatch.ConductorWorkflowID,
		JobID: dispatch.JobID, StepRunID: dispatch.StepRunID, Attempt: dispatch.Attempt,
		CapabilityKey: dispatch.CapabilityKey, CapabilityVersion: dispatch.CapabilityVersion,
		UserID: dispatch.UserID, OrganizationID: dispatch.OrganizationID, CredentialID: dispatch.CredentialID,
		ActorID: dispatch.ActorID, AuthSource: dispatch.AuthSource,
		WorkerInstanceID: dispatch.WorkerInstanceID, Status: string(dispatch.Status), InputJSON: datatypes.JSON(dispatch.InputJSON),
		LeaseExpiresAt: dispatch.LeaseExpiresAt, DeliveredAt: deliveredAt,
	}
	result := r.conn(ctx).Clauses(clause.OnConflict{DoNothing: true}).Omit("created_at").Create(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		var existing model.WorkerDispatch
		if err := r.conn(ctx).Where("conductor_task_id = ? AND attempt = ?", dispatch.ConductorTaskID, dispatch.Attempt).First(&existing).Error; err != nil {
			return nil, err
		}
		if existing.UserID != dispatch.UserID || existing.OrganizationID != dispatch.OrganizationID ||
			existing.CredentialID != dispatch.CredentialID || existing.ActorID != dispatch.ActorID ||
			existing.AuthSource != dispatch.AuthSource || existing.WorkerInstanceID != dispatch.WorkerInstanceID ||
			existing.ConductorWorkflowID != dispatch.ConductorWorkflowID || existing.JobID != dispatch.JobID ||
			existing.StepRunID != dispatch.StepRunID || existing.CapabilityKey != dispatch.CapabilityKey || existing.CapabilityVersion != dispatch.CapabilityVersion {
			return nil, domain.ErrDispatchConflict
		}
		if existing.Status == string(domain.DispatchResultReceived) || existing.Status == string(domain.DispatchFailed) {
			return nil, domain.ErrDispatchExpired
		}
		// The caller obtained and verified this exact native Task again. Refresh
		// delivery bookkeeping only; do not allocate a new task or attempt.
		if existing.Status == string(domain.DispatchExpired) || dispatch.DeliveredAt != nil && !existing.LeaseExpiresAt.After(*dispatch.DeliveredAt) {
			return r.ExtendLease(ctx, existing.ID, existing.Attempt, dispatch.LeaseExpiresAt)
		}
		return dispatchToDomain(&existing), nil
	}
	return r.Get(ctx, row.ID)
}

func (r *DispatchRepo) FindActiveManaged(ctx context.Context, actorID, instanceID, capabilityKey string, version int, now time.Time) (*domain.WorkerDispatch, error) {
	var row model.WorkerDispatch
	err := r.conn(ctx).Where("actor_id = ? AND worker_instance_id = ? AND capability_key = ? AND capability_version = ? AND auth_source = ? AND credential_id = '' AND status IN ? AND lease_expires_at > ?",
		actorID, instanceID, capabilityKey, version, "internal_runtime",
		[]string{string(domain.DispatchClaimed), string(domain.DispatchDelivered), string(domain.DispatchRunning)}, now).
		Order("created_at").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrDispatchNotFound
	}
	if err != nil {
		return nil, err
	}
	return dispatchToDomain(&row), nil
}

func (r *DispatchRepo) Get(ctx context.Context, id string) (*domain.WorkerDispatch, error) {
	var row model.WorkerDispatch
	if err := r.conn(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrDispatchNotFound
		}
		return nil, err
	}
	return dispatchToDomain(&row), nil
}

func (r *DispatchRepo) ExtendLease(ctx context.Context, id string, attempt int, leaseExpiresAt time.Time) (*domain.WorkerDispatch, error) {
	result := r.conn(ctx).Model(&model.WorkerDispatch{}).
		Where("id = ? AND attempt = ? AND status IN ?", id, attempt,
			[]string{string(domain.DispatchClaimed), string(domain.DispatchDelivered), string(domain.DispatchRunning), string(domain.DispatchExpired)}).
		Updates(map[string]any{"status": string(domain.DispatchRunning), "lease_expires_at": leaseExpiresAt})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrDispatchExpired
	}
	return r.Get(ctx, id)
}

// SaveResult is retained for archived Inbox schema compatibility only. Normal
// Worker delivery never calls it; the runtime port no longer exposes it.
func (r *DispatchRepo) SaveResult(ctx context.Context, dispatchID string, attempt int, operation, idempotencyKey string, resultJSON []byte, resultDigest string, now time.Time) (*domain.WorkerResultReceipt, error) {
	var receipt *domain.WorkerResultReceipt
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		var dispatch model.WorkerDispatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&dispatch, "id = ?", dispatchID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrDispatchNotFound
			}
			return err
		}
		if dispatch.Attempt != attempt {
			return domain.ErrDispatchConflict
		}
		if now.After(dispatch.LeaseExpiresAt) && dispatch.Status != string(domain.DispatchResultReceived) {
			return domain.ErrDispatchExpired
		}
		// The locked dispatch is the authority for terminal-result uniqueness.
		// Complete and fail must not create separate receipts for one attempt.
		if dispatch.Status == string(domain.DispatchResultReceived) {
			var existing model.WorkerResultInbox
			if err := tx.Where("dispatch_id = ? AND attempt = ?", dispatchID, attempt).First(&existing).Error; err != nil {
				return err
			}
			if existing.ResultDigest != resultDigest || existing.Operation != operation {
				return domain.ErrDispatchConflict
			}
			receipt = receiptToDomain(&existing)
			receipt.Dispatch = dispatchToDomain(&dispatch)
			return nil
		}
		row := model.WorkerResultInbox{
			ID: "receipt_" + uuid.NewString(), DispatchID: dispatchID, Attempt: attempt, Operation: operation,
			IdempotencyKey: idempotencyKey, ResultJSON: datatypes.JSON(resultJSON), ResultDigest: resultDigest,
			ConductorStatus: "pending",
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Omit("created_at", "submitted_at").Create(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			// GORM includes a nonzero primary key in First's predicate. The new
			// receipt UUID is not the existing receipt's ID after ON CONFLICT.
			row = model.WorkerResultInbox{}
			if err := tx.Where("dispatch_id = ? AND (idempotency_key = ? OR (attempt = ? AND operation = ?))", dispatchID, idempotencyKey, attempt, operation).First(&row).Error; err != nil {
				return err
			}
			if row.ResultDigest != resultDigest || row.Operation != operation {
				return domain.ErrDispatchConflict
			}
		} else {
			completedAt := now
			if err := tx.Model(&dispatch).Updates(map[string]any{"status": string(domain.DispatchResultReceived), "completed_at": completedAt}).Error; err != nil {
				return err
			}
		}
		receipt = receiptToDomain(&row)
		receipt.Dispatch = dispatchToDomain(&dispatch)
		return nil
	})
	return receipt, err
}

// Delivery bookkeeping only; output and execution state live in Conductor.
func (r *DispatchRepo) MarkResultReceived(ctx context.Context, id string, attempt int, at time.Time) error {
	return r.conn(ctx).Model(&model.WorkerDispatch{}).Where("id = ? AND attempt = ?", id, attempt).
		Updates(map[string]any{"status": string(domain.DispatchResultReceived), "completed_at": at}).Error
}

func (r *DispatchRepo) ListPendingResults(ctx context.Context, limit int) ([]*domain.WorkerResultReceipt, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []model.WorkerResultInbox
	if err := r.conn(ctx).Where("conductor_status IN ?", []string{"pending", "failed"}).Order("created_at").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]*domain.WorkerResultReceipt, 0, len(rows))
	for i := range rows {
		receipt := receiptToDomain(&rows[i])
		dispatch, err := r.Get(ctx, receipt.DispatchID)
		if err != nil {
			return nil, err
		}
		receipt.Dispatch = dispatch
		result = append(result, receipt)
	}
	return result, nil
}

func (r *DispatchRepo) MarkResultSubmitted(ctx context.Context, receiptID string, at time.Time) error {
	return r.conn(ctx).Model(&model.WorkerResultInbox{}).Where("id = ?", receiptID).
		Updates(map[string]any{"conductor_status": "submitted", "submitted_at": at, "last_error": ""}).Error
}

func (r *DispatchRepo) MarkResultFailed(ctx context.Context, receiptID, message string) error {
	return r.conn(ctx).Model(&model.WorkerResultInbox{}).Where("id = ?", receiptID).
		Updates(map[string]any{"conductor_status": "failed", "last_error": message}).Error
}

func dispatchToDomain(row *model.WorkerDispatch) *domain.WorkerDispatch {
	return &domain.WorkerDispatch{
		ID: row.ID, ConductorTaskID: row.ConductorTaskID, ConductorWorkflowID: row.ConductorWorkflowID,
		JobID: row.JobID, StepRunID: row.StepRunID, Attempt: row.Attempt,
		CapabilityKey: row.CapabilityKey, CapabilityVersion: row.CapabilityVersion,
		UserID: row.UserID, OrganizationID: row.OrganizationID, CredentialID: row.CredentialID,
		ActorID: row.ActorID, AuthSource: row.AuthSource,
		WorkerInstanceID: row.WorkerInstanceID, Status: domain.DispatchStatus(row.Status), InputJSON: []byte(row.InputJSON),
		LeaseExpiresAt: row.LeaseExpiresAt, CreatedAt: row.CreatedAt, DeliveredAt: row.DeliveredAt, CompletedAt: row.CompletedAt,
	}
}

func receiptToDomain(row *model.WorkerResultInbox) *domain.WorkerResultReceipt {
	return &domain.WorkerResultReceipt{
		ID: row.ID, DispatchID: row.DispatchID, Attempt: row.Attempt, Operation: row.Operation,
		IdempotencyKey: row.IdempotencyKey, ResultJSON: []byte(row.ResultJSON), ResultDigest: row.ResultDigest,
		ConductorStatus: row.ConductorStatus, LastError: row.LastError, CreatedAt: row.CreatedAt, SubmittedAt: row.SubmittedAt,
	}
}
