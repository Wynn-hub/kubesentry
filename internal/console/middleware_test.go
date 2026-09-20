package console

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

// An inbound X-Request-Id must be reused verbatim so one trace spans the whole
// request chain rather than restarting at the console.
func TestRequestContextReusesInboundTraceID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/policies", nil)
	req.Header.Set("X-Request-Id", "inbound-trace-42")
	seen := TraceIDFromContext(requestContext(req))
	if seen != "inbound-trace-42" {
		t.Fatalf("trace id = %q, want inbound-trace-42", seen)
	}
}

// With no inbound header a fresh id must be minted, so every request stays
// traceable even when the caller sends nothing.
func TestRequestContextMintsTraceIDWhenAbsent(t *testing.T) {
	if err := dbmodel.Init(1); err != nil {
		t.Fatalf("dbmodel.Init: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/v1/policies", nil)
	seen := TraceIDFromContext(requestContext(req))
	if seen == "" {
		t.Fatal("trace id is empty, want a minted id")
	}
}

// TraceIDFromContext must not panic on a context that never passed through the
// middleware — service code is also reachable from tests and background jobs.
func TestTraceIDFromContextEmptyWhenUnset(t *testing.T) {
	if got := TraceIDFromContext(context.Background()); got != "" {
		t.Fatalf("trace id = %q, want empty", got)
	}
}
