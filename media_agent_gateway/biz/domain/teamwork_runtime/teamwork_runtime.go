package teamwork_runtime

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound          = errors.New("allowed topology version not found")
	ErrNotAllowed        = errors.New("topology version is not allowed")
	ErrWorkspaceMismatch = errors.New("topology workspace ownership cannot change")
)

type Event struct {
	ID, WorkspaceID, TopologyID, Status string
	TopologyVersion                     int64
}

type Version struct {
	TopologyID, WorkspaceID, Status string
	Version                         int64
	UpdatedAt                       time.Time
}

type ApplyResult struct {
	Duplicate bool
	Advanced  bool
}

type Repository interface {
	ApplyEvent(context.Context, Event, time.Time) (ApplyResult, error)
	Get(context.Context, string) (*Version, error)
}

func ValidateEvent(id, workspaceID, topologyID string, version int64, status string) (Event, error) {
	event := Event{ID: strings.TrimSpace(id), WorkspaceID: strings.TrimSpace(workspaceID), TopologyID: strings.TrimSpace(topologyID), TopologyVersion: version, Status: strings.TrimSpace(status)}
	if event.ID == "" || event.WorkspaceID == "" || event.TopologyID == "" || event.TopologyVersion <= 0 {
		return Event{}, errors.New("event_id, workspace_id, topology_id and positive topology_version are required")
	}
	if event.Status != "active" && event.Status != "archived" && event.Status != "deleted" {
		return Event{}, errors.New("invalid topology status")
	}
	return event, nil
}

func (v Version) Allows(workspaceID string, version int64) bool {
	return v.WorkspaceID == workspaceID && v.Status == "active" && version > 0 && v.Version == version
}
