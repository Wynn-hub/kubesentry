// internal/console/traceid.go
package console

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// TraceIDFromRequest returns a per-request correlation id: reuses an inbound
// X-Request-Id header when present, otherwise mints a fresh snowflake id.
func TraceIDFromRequest(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	return strconv.FormatInt(dbmodel.NewID(), 10)
}

// notifyChange best-effort notifies the gateway that topic changed, so its
// pushLoop wakes immediately instead of waiting out the 60s resync fallback
// (spec D12). h.Gateway is nil when GATEWAY_ADDRESS wasn't configured at
// startup (soft dependency) — skip silently rather than panic. A failed or
// skipped call never fails the HTTP request: the write already committed,
// and the periodic resync corrects any missed notification.
func (h *Handlers) notifyChange(r *http.Request, topic string) {
	if h.Gateway == nil {
		return
	}
	traceID := TraceIDFromRequest(r)
	req := &gatewaypb.NotifyChangeRequest{Topic: topic, TraceId: traceID}
	if _, err := h.Gateway.NotifyChange(r.Context(), req); err != nil {
		slog.Warn("notify gateway of change", "topic", topic, "trace_id", traceID, "error", err)
	}
}
