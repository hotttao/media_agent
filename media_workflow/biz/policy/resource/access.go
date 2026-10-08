// Package resource evaluates existing resource visibility and department grants.
// Membership facts must be supplied by the authenticated application boundary.
package resource

type Level int

const (
	None Level = iota
	Viewer
	Editor
	Owner
)

type Grant struct{ TeamID, Permission string }

func Effective(owner, visibility, caller string, teams []string, grants []Grant) Level {
	if caller == "" {
		return None
	}
	if owner != "" && owner == caller {
		return Owner
	}
	if visibility == "private" {
		return None
	}
	level := None
	if visibility == "public" {
		level = Viewer
	}
	if visibility != "team" && visibility != "public" {
		return None
	}
	members := map[string]bool{}
	for _, team := range teams {
		if team != "" {
			members[team] = true
		}
	}
	for _, grant := range grants {
		if !members[grant.TeamID] {
			continue
		}
		if grant.Permission == "editor" {
			return Editor
		}
		if grant.Permission == "viewer" {
			level = Viewer
		}
	}
	return level
}
