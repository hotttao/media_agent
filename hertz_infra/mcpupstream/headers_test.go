package mcpupstream

import "testing"

func TestCredentialHeaderCannotReplaceProtocolOrTrustedIdentity(t *testing.T) {
	for _, name := range []string{"X-Metadata-Gateway-Credential", "X-Session-Token", "X-Platform-Caller", "Host", "cookie", "Authorization", "Content-Type", "Connection", "Transfer-Encoding", "Mcp-Session-Id", "X-Media-User-ID", "X-Agent-Instance-Credential", "X-Control-Service-Token", "X-MCP-Gateway-Credential", "Proxy-Authorization", "X-Forwarded-Host", "bad\r\nheader", ""} {
		if ValidCredentialHeader(name) {
			t.Fatalf("accepted reserved header %q", name)
		}
	}
	for _, name := range []string{"X-API-Key", "api-key", "X-Subscription-Key"} {
		if !ValidCredentialHeader(name) {
			t.Fatalf("rejected API Key header %q", name)
		}
	}
}
