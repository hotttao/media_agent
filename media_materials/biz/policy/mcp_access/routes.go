package mcp_access

import "strings"

func Allowed(operation, method, path string) bool {
	if method != "GET" || !strings.HasPrefix(path, "/api/v1/") || strings.ContainsAny(path, "%\\?#") || strings.Contains(path, "..") {
		return false
	}
	if operation != "jobs_submit" && operation != "jobs_get" {
		return false
	}
	if path == "/api/v1/library-objects:resolve" {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	if len(parts) != 2 || parts[1] == "" {
		return false
	}
	if operation == "jobs_get" {
		return parts[0] == "generated-assets" && parts[1] != "by-source"
	}
	return parts[0] == "products" || parts[0] == "virtual-ips"
}
