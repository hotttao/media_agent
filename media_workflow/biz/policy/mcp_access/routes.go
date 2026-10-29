package mcp_access

import "strings"

// Allowed is a closed mapping from the delegated tool operation to public APIs.
func Allowed(operation, method, path string) bool {
	if !strings.HasPrefix(path, "/api/v1/") || strings.ContainsAny(path, "%\\?#") || strings.Contains(path, "..") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	kind := parts[0]
	detail := len(parts) == 2
	version := len(parts) == 4 && parts[2] == "versions"
	if method == "GET" {
		switch operation {
		case "templates_list":
			return kind == "templates" && len(parts) == 1
		case "templates_get", "templates_publish":
			return kind == "templates" && (detail || version)
		case "templates_create", "capabilities_list":
			return kind == "capabilities" && (len(parts) == 1 || version)
		case "workflows_list":
			return kind == "workflows" && len(parts) == 1
		case "workflows_get":
			return kind == "workflows" && (detail || version)
		case "workflows_create", "workflows_validate", "workflows_publish", "jobs_submit":
			return (kind == "templates" || kind == "workflows") && (detail || version) || kind == "capabilities" && version
		case "jobs_list":
			return kind == "jobs" && len(parts) == 1
		case "jobs_get":
			return kind == "jobs" && detail
		}
		return false
	}
	if method != "POST" {
		return false
	}
	switch operation {
	case "templates_create":
		return path == "/api/v1/templates"
	case "templates_publish":
		return kind == "templates" && len(parts) == 3 && parts[2] == "publish"
	case "workflows_create":
		return path == "/api/v1/workflows"
	case "workflows_validate":
		return path == "/api/v1/workflows/validate"
	case "workflows_publish":
		return kind == "workflows" && len(parts) == 3 && parts[2] == "publish"
	case "jobs_submit":
		return path == "/api/v1/jobs"
	}
	return false
}
