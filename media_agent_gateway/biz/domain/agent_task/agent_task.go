package agent_task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("agent task not found")
	ErrForbidden = errors.New("agent task does not belong to user")
	ErrConflict  = errors.New("agent task state conflict")
	ErrCancelled = errors.New("agent task was cancelled during dispatch")
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusWaitingTurn Status = "waiting_turn"
	StatusRetryWait   Status = "retry_wait"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
)

type Task struct {
	ID, WorkspaceID, RequestedBy, SessionID        string
	TopologyID                                     string
	TopologyVersion                                int64
	Source, SourceResourceID, IdempotencyKey       string
	InputJSON                                      string
	Status                                         Status
	Attempt                                        int32
	TurnID, ResultRefJSON, ErrorCode, ErrorMessage string
	CreatedAt, UpdatedAt                           time.Time
	StartedAt, FinishedAt                          *time.Time
}

type Repository interface {
	Create(context.Context, *Task) error
	FindByKey(context.Context, string, string, string, string) (*Task, error)
	ListOwned(context.Context, string, string, Status) ([]Task, error)
	GetOwned(context.Context, string, string) (*Task, error)
	Cancel(context.Context, *Task, []Status) error
}

// DispatchRepository persists the Task side of the cross-system dispatch workflow.
// Every transition is a compare-and-swap so duplicate queue delivery is harmless.
type DispatchRepository interface {
	Get(context.Context, string) (*Task, error)
	Claim(context.Context, string, time.Time) (*Task, error)
	MarkWaitingTurn(context.Context, string, string, time.Time) error
	MarkRetryWait(context.Context, string, string, string, time.Time) error
	MarkTerminal(context.Context, string, Status, string, string, string, time.Time) error
	ListWaitingTurn(context.Context, int) ([]Task, error)
}
type TopologyCancellationRepository interface {
	CancelStaleTopology(context.Context, string, int64, time.Time) ([]Task, error)
}

type OutboxEvent struct {
	ID, AggregateID, EventType, PayloadJSON string
	Attempts                                int32
	AvailableAt, CreatedAt, UpdatedAt       time.Time
}

type OutboxRepository interface {
	Enqueue(context.Context, *OutboxEvent) error
	ListPendingActions(context.Context, int) ([]OutboxEvent, error)
	MarkDelivered(context.Context, string, time.Time) error
	MarkDeliveryFailed(context.Context, string, string, time.Time) error
}

func Validate(workspaceID, source, sourceResourceID, key, input string) (string, string, string, string, string, error) {
	workspaceID, source, sourceResourceID, key = strings.TrimSpace(workspaceID), strings.TrimSpace(source), strings.TrimSpace(sourceResourceID), strings.TrimSpace(key)
	if workspaceID == "" || source == "" || sourceResourceID == "" || key == "" {
		return "", "", "", "", "", errors.New("workspace_id, source, source_resource_id and idempotency_key are required")
	}
	input = strings.TrimSpace(input)
	if input == "" {
		input = "{}"
	}
	var value any
	if err := json.Unmarshal([]byte(input), &value); err != nil {
		return "", "", "", "", "", fmt.Errorf("input_json must be valid JSON: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return "", "", "", "", "", errors.New("input_json must be a JSON object")
	}
	return workspaceID, source, sourceResourceID, key, input, nil
}

func ValidateTopology(topologyID string, topologyVersion int64) (string, int64, error) {
	topologyID = strings.TrimSpace(topologyID)
	if topologyID == "" && topologyVersion == 0 {
		return "", 0, nil
	}
	if topologyID == "" || topologyVersion <= 0 {
		return "", 0, errors.New("topology_id and positive topology_version must be provided together")
	}
	return topologyID, topologyVersion, nil
}
func ParseStatus(value string) (Status, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	s := Status(strings.TrimSpace(value))
	switch s {
	case StatusQueued, StatusRunning, StatusWaitingTurn, StatusRetryWait, StatusCompleted, StatusFailed, StatusCancelled:
		return s, nil
	default:
		return "", fmt.Errorf("invalid task status %q", value)
	}
}
func IsTerminal(s Status) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}
