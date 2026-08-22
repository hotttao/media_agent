package batch_run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("batch run not found")
	ErrForbidden = errors.New("batch run does not belong to user")
	ErrConflict  = errors.New("batch run state conflict")
)

type Status string

const (
	StatusCreating      Status = "creating"
	StatusRunning       Status = "running"
	StatusCompleted     Status = "completed"
	StatusPartialFailed Status = "partial_failed"
	StatusFailed        Status = "failed"
	StatusCancelled     Status = "cancelled"
)

type ItemStatus string

const (
	ItemPending   ItemStatus = "pending"
	ItemQueued    ItemStatus = "queued"
	ItemRunning   ItemStatus = "running"
	ItemCompleted ItemStatus = "completed"
	ItemFailed    ItemStatus = "failed"
	ItemCancelled ItemStatus = "cancelled"
)

type Item struct {
	ItemKey, SessionID, AgentTaskID, InputJSON, ErrorCode string
	Status                                                ItemStatus
	CreatedAt, UpdatedAt                                  time.Time
}

type BatchRun struct {
	ID, WorkspaceID, RequestedBy, IdempotencyKey string
	Status                                       Status
	ItemCount, NextCursor                        int32
	CompletedCount, FailedCount, CancelledCount  int32
	Items                                        []Item
	CreatedAt, UpdatedAt                         time.Time
	FinishedAt                                   *time.Time
}

type Repository interface {
	Create(context.Context, *BatchRun) error
	FindByKey(context.Context, string, string, string) (*BatchRun, error)
	ListOwned(context.Context, string, string, Status) ([]BatchRun, error)
	GetOwned(context.Context, string, string) (*BatchRun, error)
	Cancel(context.Context, *BatchRun) error
}

type OutboxEvent struct {
	ID, BatchRunID, Action, PayloadJSON string
	Attempts                            int32
	AvailableAt, CreatedAt, UpdatedAt   time.Time
}

type OutboxRepository interface {
	Enqueue(context.Context, *OutboxEvent) error
	ListPendingActions(context.Context, int) ([]OutboxEvent, error)
	MarkDelivered(context.Context, string, time.Time) error
	MarkDeliveryFailed(context.Context, string, string, time.Time) error
}

type DispatchRepository interface {
	Get(context.Context, string) (*BatchRun, error)
	AttachTask(context.Context, string, int32, string, string, time.Time) error
	ApplyTaskTerminal(context.Context, TaskTerminal, time.Time) error
}

type TaskTerminal struct {
	EventID, TaskID, Status, ErrorCode string
}

type TaskTerminalSource interface {
	ListPendingBatchTerminal(context.Context, int) ([]TaskTerminal, error)
	MarkDelivered(context.Context, string, time.Time) error
}

func Validate(workspaceID, key string, items []Item) (string, string, []Item, error) {
	workspaceID, key = strings.TrimSpace(workspaceID), strings.TrimSpace(key)
	if workspaceID == "" || key == "" {
		return "", "", nil, errors.New("workspace_id and idempotency_key are required")
	}
	if len(items) < 1 || len(items) > 1000 {
		return "", "", nil, errors.New("items must contain between 1 and 1000 entries")
	}
	seen := make(map[string]struct{}, len(items))
	normalized := make([]Item, 0, len(items))
	for _, item := range items {
		item.ItemKey, item.SessionID, item.InputJSON = strings.TrimSpace(item.ItemKey), strings.TrimSpace(item.SessionID), strings.TrimSpace(item.InputJSON)
		if item.ItemKey == "" || item.SessionID == "" {
			return "", "", nil, errors.New("item_key and session_id are required for every item")
		}
		if _, ok := seen[item.ItemKey]; ok {
			return "", "", nil, fmt.Errorf("duplicate item_key %q", item.ItemKey)
		}
		seen[item.ItemKey] = struct{}{}
		if item.InputJSON == "" {
			item.InputJSON = "{}"
		}
		var value any
		if err := json.Unmarshal([]byte(item.InputJSON), &value); err != nil {
			return "", "", nil, fmt.Errorf("item %q input_json must be valid JSON: %w", item.ItemKey, err)
		}
		if _, ok := value.(map[string]any); !ok {
			return "", "", nil, fmt.Errorf("item %q input_json must be a JSON object", item.ItemKey)
		}
		normalized = append(normalized, item)
	}
	return workspaceID, key, normalized, nil
}

func ParseStatus(value string) (Status, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	status := Status(strings.TrimSpace(value))
	switch status {
	case StatusCreating, StatusRunning, StatusCompleted, StatusPartialFailed, StatusFailed, StatusCancelled:
		return status, nil
	default:
		return "", fmt.Errorf("invalid Batch Run status %q", value)
	}
}
