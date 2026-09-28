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
		return dispatchToDomain(&existing), nil
	}
	return r.Get(ctx, row.ID)
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
		Where("id = ? AND attempt = ? AND status IN ? AND lease_expires_at > now()", id, attempt,
			[]string{string(domain.DispatchClaimed), string(domain.DispatchDelivered), string(domain.DispatchRunning)}).
		Updates(map[string]any{"status": string(domain.DispatchRunning), "lease_expires_at": leaseExpiresAt})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, domain.ErrDispatchExpired
	}
	return r.Get(ctx, id)
}

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

func (r *DispatchRepo) ExpireLeases(ctx context.Context, now time.Time) (int64, error) {
	result := r.conn(ctx).Model(&model.WorkerDispatch{}).
		Where("status IN ? AND lease_expires_at <= ?", []string{
			string(domain.DispatchClaimed), string(domain.DispatchDelivered), string(domain.DispatchRunning),
		}, now).
		Update("status", string(domain.DispatchExpired))
	return result.RowsAffected, result.Error
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
