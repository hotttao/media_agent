package mcpupstream

import (
	"regexp"
	"slices"
	"strings"
)

var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// ValidCredentialHeader excludes protocol and trusted identity headers.
func ValidCredentialHeader(name string) bool {
	if len(name) > 128 || !headerName.MatchString(name) {
		return false
	}
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "x-metadata-") || strings.HasPrefix(name, "x-platform-") || strings.HasPrefix(name, "x-media-") || strings.HasPrefix(name, "x-agent-") || strings.HasPrefix(name, "x-control-") || strings.HasPrefix(name, "x-mcp-") || strings.HasPrefix(name, "proxy-") {
		return false
	}
	return !slices.Contains([]string{"x-session-token", "host", "cookie", "set-cookie", "connection", "keep-alive", "te", "trailer", "transfer-encoding", "upgrade", "content-type", "content-length", "accept", "authorization", "mcp-session-id", "mcp-protocol-version", "origin", "forwarded", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto"}, name)
}
