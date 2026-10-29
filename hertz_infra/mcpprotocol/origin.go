package mcpprotocol

import "net/http"

// WithOrigins enforces an exact deployment allowlist for browser-originated
// requests. Non-browser MCP clients normally omit Origin.
func WithOrigins(next http.Handler, origins []string) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range origins {
		if origin != "" && origin != "*" && origin != "null" {
			allowed[origin] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && !allowed[origin] {
			http.Error(w, "MCP origin denied", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
