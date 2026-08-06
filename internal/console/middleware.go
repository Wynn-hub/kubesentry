package console

import (
	"context"

	"github.com/ogen-go/ogen/middleware"
)

// traceIDKey is the context key carrying the per-request correlation id.
// Unexported so no other package can collide with it.
type traceIDKey struct{}

// TraceIDMiddleware puts the request's correlation id into the context.
//
// ogen's generated handlers receive only (ctx, params) — there is no
// *http.Request to read a header from (see oas_server_gen.go). The raw request
// is reachable here, in middleware, so this is where the id is extracted and
// handed down. Service code then reads it via TraceIDFromContext instead of
// taking an *http.Request parameter, which keeps the service layer free of
// HTTP types.
func TraceIDMiddleware() middleware.Middleware {
	return func(
		req middleware.Request,
		next middleware.Next,
	) (middleware.Response, error) {
		// Raw is documented as always set for HTTP servers, but a synthetic or
		// client-side invocation would otherwise panic here.
		traceID := ""
		if req.Raw != nil {
			traceID = TraceIDFromRequest(req.Raw)
		}
		if traceID != "" {
			req.SetContext(context.WithValue(req.Context, traceIDKey{}, traceID))
		}
		return next(req)
	}
}

// TraceIDFromContext returns the correlation id TraceIDMiddleware stored, or
// "" when the context never passed through it (tests, background jobs).
func TraceIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}
