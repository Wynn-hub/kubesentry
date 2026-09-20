package console

import (
	"context"
)

// traceIDKey is the context key carrying the per-request correlation id.
// Unexported so no other package can collide with it.
type traceIDKey struct{}

// TraceIDFromContext returns the request correlation id, or "" for contexts
// created outside the HTTP adapter (tests and background jobs).
func TraceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}
