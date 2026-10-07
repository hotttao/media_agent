package gateway

import (
	"context"
	"encoding/hex"
	"strings"
)

type taskTraceKey struct{}

// Only canonical W3C version 00 is accepted. Metadata is correlation, never
// credentials or identity; malformed legacy envelopes simply omit it.
func validTraceParent(parent string) bool {
	if len(parent) != 55 || !strings.HasPrefix(parent, "00-") || parent[35] != '-' || parent[52] != '-' || strings.ToLower(parent) != parent {
		return false
	}
	for _, part := range []string{parent[3:35], parent[36:52], parent[53:55]} {
		if _, err := hex.DecodeString(part); err != nil {
			return false
		}
	}
	return parent[3:35] != strings.Repeat("0", 32) && parent[36:52] != strings.Repeat("0", 16)
}

func TaskContext(ctx context.Context, task Task) context.Context {
	if validTraceParent(task.TraceParent) {
		return context.WithValue(ctx, taskTraceKey{}, task.TraceParent)
	}
	return ctx
}

func (task Task) TraceID() string {
	if validTraceParent(task.TraceParent) {
		return task.TraceParent[3:35]
	}
	return ""
}
