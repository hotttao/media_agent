package schedule

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("schedule not found")
	ErrConflict = errors.New("schedule version conflict")
)

type Status string

const (
	StatusActive  Status = "active"
	StatusPaused  Status = "paused"
	StatusDeleted Status = "deleted"
)

type Schedule struct {
	ID, WorkspaceID, RequestedBy, AgentConfigID, Rule, Timezone string
	Status                                                      Status
	NextRunAt                                                   *time.Time
	Version                                                     int64
	CreatedAt, UpdatedAt                                        time.Time
}
type Trigger struct {
	ID, ScheduleID, Status, AgentTaskID, ErrorCode string
	DueAt, CreatedAt, UpdatedAt                    time.Time
}
type Repository interface {
	Create(context.Context, *Schedule) error
	ListOwned(context.Context, string, string) ([]Schedule, error)
	GetOwned(context.Context, string, string) (*Schedule, error)
	Update(context.Context, *Schedule, int64) error
	Delete(context.Context, *Schedule, int64) error
	CreateTrigger(context.Context, *Trigger) error
	GetTrigger(context.Context, string) (*Trigger, error)
	UpdateTrigger(context.Context, *Trigger, string) error
}

func Validate(rule, tz string) (string, string, error) {
	rule = strings.TrimSpace(rule)
	tz = strings.TrimSpace(tz)
	if rule == "" || tz == "" {
		return "", "", errors.New("rule and timezone are required")
	}
	return rule, tz, nil
}
