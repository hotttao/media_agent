package job

import (
	"context"
	"errors"
	"time"
)

type DispatchStatus string

const (
	DispatchClaimed        DispatchStatus = "claimed"
	DispatchDelivered      DispatchStatus = "delivered"
	DispatchRunning        DispatchStatus = "running"
	DispatchResultReceived DispatchStatus = "result_received"
	DispatchExpired        DispatchStatus = "expired"
	DispatchFailed         DispatchStatus = "failed"
)

type WorkerDispatch struct {
	ID, ConductorTaskID, ConductorWorkflowID string
	JobID, StepRunID                         string
	Attempt                                  int
	CapabilityKey                            string
	CapabilityVersion                        int
	UserID, OrganizationID, CredentialID     string
	ActorID, AuthSource                      string
	WorkerInstanceID                         string
	Status                                   DispatchStatus
	InputJSON                                []byte
	LeaseExpiresAt, CreatedAt                time.Time
	DeliveredAt, CompletedAt                 *time.Time
}

type WorkerResultReceipt struct {
	ID, DispatchID, Operation, IdempotencyKey string
	Attempt                                   int
	ResultJSON                                []byte
	ResultDigest, ConductorStatus, LastError  string
	CreatedAt                                 time.Time
	SubmittedAt                               *time.Time
	Dispatch                                  *WorkerDispatch
}

type DispatchRepo interface {
	FindActive(ctx context.Context, userID, organizationID, credentialID, workerInstanceID, capabilityKey string, capabilityVersion int, now time.Time) (*WorkerDispatch, error)
	FindActiveManaged(ctx context.Context, actorID, workerInstanceID, capabilityKey string, capabilityVersion int, now time.Time) (*WorkerDispatch, error)
	Create(ctx context.Context, dispatch *WorkerDispatch) (*WorkerDispatch, error)
	Get(ctx context.Context, id string) (*WorkerDispatch, error)
	ExtendLease(ctx context.Context, id string, attempt int, leaseExpiresAt time.Time) (*WorkerDispatch, error)
	MarkResultReceived(ctx context.Context, id string, attempt int, at time.Time) error
}

var (
	ErrDispatchNotFound = errors.New("worker dispatch not found")
	ErrDispatchConflict = errors.New("worker dispatch conflict")
	ErrDispatchExpired  = errors.New("worker task no longer active")
)
