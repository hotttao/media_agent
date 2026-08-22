package teamwork

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("teamwork topology not found")
	ErrConflict = errors.New("teamwork topology version conflict")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
	StatusDeleted  Status = "deleted"
)

type Member struct {
	MemberID      string `json:"member_id"`
	AgentConfigID string `json:"agent_config_id"`
	Name          string `json:"name"`
}
type Edge struct {
	FromMemberID  string         `json:"from_member_id"`
	ToMemberID    string         `json:"to_member_id"`
	Mode          string         `json:"mode"`
	Configuration map[string]any `json:"configuration,omitempty"`
}
type Topology struct {
	ID, WorkspaceID, Name, EntryMemberID string
	Members                              []Member
	Edges                                []Edge
	Version                              int64
	Status                               Status
	CreatedAt, UpdatedAt                 time.Time
}
type Repository interface {
	Create(context.Context, *Topology) error
	ListOwned(context.Context, string, string) ([]Topology, error)
	GetOwned(context.Context, string, string) (*Topology, error)
	Update(context.Context, *Topology, int64) error
	Delete(context.Context, *Topology, int64) error
}

func Validate(name, entry, membersJSON, edgesJSON string) (string, string, []Member, []Edge, error) {
	name, entry = strings.TrimSpace(name), strings.TrimSpace(entry)
	if name == "" || entry == "" {
		return "", "", nil, nil, errors.New("name and entry_member_id are required")
	}
	var members []Member
	if err := json.Unmarshal([]byte(membersJSON), &members); err != nil || len(members) == 0 {
		return "", "", nil, nil, errors.New("members_json must be a non-empty JSON array")
	}
	seen := map[string]bool{}
	for _, m := range members {
		m.MemberID = strings.TrimSpace(m.MemberID)
		m.AgentConfigID = strings.TrimSpace(m.AgentConfigID)
		m.Name = strings.TrimSpace(m.Name)
		if m.MemberID == "" || m.AgentConfigID == "" || m.Name == "" || seen[m.MemberID] {
			return "", "", nil, nil, errors.New("members must have unique non-empty member_id, agent_config_id and name")
		}
		seen[m.MemberID] = true
	}
	if !seen[entry] {
		return "", "", nil, nil, errors.New("entry_member_id must reference a member")
	}
	var edges []Edge
	if strings.TrimSpace(edgesJSON) != "" {
		if err := json.Unmarshal([]byte(edgesJSON), &edges); err != nil {
			return "", "", nil, nil, errors.New("edges_json must be a JSON array")
		}
	}
	for _, e := range edges {
		if e.FromMemberID == e.ToMemberID || !seen[e.FromMemberID] || !seen[e.ToMemberID] || strings.TrimSpace(e.Mode) == "" {
			return "", "", nil, nil, errors.New("edges must reference distinct members and a mode")
		}
	}
	return name, entry, members, edges, nil
}
func Encode(v any) string { b, _ := json.Marshal(v); return string(b) }
