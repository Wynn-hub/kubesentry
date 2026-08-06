package console

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ogen-go/ogen/middleware"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

// An inbound X-Request-Id must be reused verbatim so one trace spans the whole
// request chain rather than restarting at the console.
func TestTraceIDMiddlewareReusesInboundHeader(t *testing.T) {
	var seen string
	req := httptest.NewRequest("GET", "/api/v1/policies", nil)
	req.Header.Set("X-Request-Id", "inbound-trace-42")

	_, err := TraceIDMiddleware()(
		middleware.Request{Context: context.Background(), Raw: req},
		func(r middleware.Request) (middleware.Response, error) {
			seen = TraceIDFromContext(r.Context)
			return middleware.Response{}, nil
		},
	)
	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if seen != "inbound-trace-42" {
		t.Fatalf("trace id = %q, want inbound-trace-42", seen)
	}
}

// With no inbound header a fresh id must be minted, so every request stays
// traceable even when the caller sends nothing.
func TestTraceIDMiddlewareMintsWhenAbsent(t *testing.T) {
	if err := dbmodel.Init(1); err != nil {
		t.Fatalf("dbmodel.Init: %v", err)
	}
	var seen string
	req := httptest.NewRequest("GET", "/api/v1/policies", nil)

	if _, err := TraceIDMiddleware()(
		middleware.Request{Context: context.Background(), Raw: req},
		func(r middleware.Request) (middleware.Response, error) {
			seen = TraceIDFromContext(r.Context)
			return middleware.Response{}, nil
		},
	); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
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

// A nil Raw request must not panic. ogen documents Raw as always set for HTTP
// servers, but a synthetic invocation should not take the process down.
func TestTraceIDMiddlewareToleratesNilRaw(t *testing.T) {
	called := false
	if _, err := TraceIDMiddleware()(
		middleware.Request{Context: context.Background()},
		func(r middleware.Request) (middleware.Response, error) {
			called = true
			return middleware.Response{}, nil
		},
	); err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if !called {
		t.Fatal("next was not called")
	}
}
