package resource

// Access is a read projection, not an authorization receipt for future writes.
type Access struct {
	MyPermission   string
	AllowedActions []string
}

func Actions(level Level, archived, authoring, shareable, runnable bool) Access {
	result := Access{AllowedActions: []string{}}
	switch level {
	case Owner:
		result.MyPermission = "owner"
	case Editor:
		result.MyPermission = "editor"
	case Viewer:
		result.MyPermission = "viewer"
	default:
		return result
	}
	result.AllowedActions = append(result.AllowedActions, "read")
	if level >= Editor {
		if !archived && authoring {
			result.AllowedActions = append(result.AllowedActions, "update")
		}
		if !archived {
			result.AllowedActions = append(result.AllowedActions, "archive")
		}
	}
	if level == Owner {
		if !archived && authoring {
			result.AllowedActions = append(result.AllowedActions, "publish")
		}
		if shareable {
			result.AllowedActions = append(result.AllowedActions, "change_visibility", "grant")
		}
		result.AllowedActions = append(result.AllowedActions, "revoke")
	}
	if !archived && authoring && runnable {
		result.AllowedActions = append(result.AllowedActions, "run")
	}
	return result
}
