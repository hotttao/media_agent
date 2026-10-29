package mcpprotocol

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginRejectedBeforeAuthentication(t *testing.T) {
	for _, origin := range []string{"", "https://ui.example", "https://evil.example", "null"} {
		called := false
		h := WithOrigins(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }), []string{"https://ui.example"})
		r := httptest.NewRequest("POST", "http://gateway/mcp", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		expected := origin == "" || origin == "https://ui.example"
		if called != expected {
			t.Fatalf("origin %q entered auth=%v", origin, called)
		}
	}
}
