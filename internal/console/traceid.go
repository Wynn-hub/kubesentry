// internal/console/traceid.go
package console

import (
	"net/http"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

// TraceIDFromRequest returns a per-request correlation id: reuses an inbound
// X-Request-Id header when present, otherwise mints a fresh snowflake id.
func TraceIDFromRequest(r *http.Request) string {
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return id
	}
	return strconv.FormatInt(dbmodel.NewID(), 10)
}
