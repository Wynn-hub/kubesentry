package console

import (
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"

	"github.com/Wynn-hub/kubesentry/internal/console/handler"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// Handlers holds dependencies shared by all API handlers.
type Handlers struct {
	// Repos is the GORM data-access layer backing the policy/group/exception/
	// summary handlers (Task 9 + Task 10). Discovery is still used by
	// handlers_schema.go, the one remaining handler that legitimately talks
	// to a live cluster (OpenAPI field schema for the visual policy builder)
	// rather than the database — out of scope for this task.
	Repos         repository.Repositories
	Discovery     discoveryInterface
	SchemaFetcher schemaFetcher // optional override for tests; nil uses discoverySchemaFetcher

	// Gateway is the gRPC client used to notify the gateway of writes (see
	// notifyChange in traceid.go) so its pushLoop wakes without waiting for
	// the 60s resync fallback. nil when GATEWAY_ADDRESS wasn't configured at
	// startup — every write handler must tolerate that (soft dependency).
	Gateway gatewaypb.HubClient

	schemaCacheOnce sync.Once
	schemaCacheMap  *sync.Map
}

// Server is the console HTTP server (plain HTTP; accessed via port-forward,
// or fronted by the nginx `web` container which proxies /api/* and /healthz
// here and serves the built SPA itself — see charts/kubesentry-gateway/console).
type Server struct {
	Handler http.Handler
	mux     *http.ServeMux
}

// NewServer wires the REST API only; the SPA is served by a separate nginx
// container (see Dockerfile.console-web and the gateway chart's nginx ConfigMap).
// ready reports whether console-api's dependencies (currently: the database)
// are reachable; nil always reports ready.
func NewServer(h *Handlers, ready func() bool) (*Server, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready == nil || ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("GET /api/docs/openapi.json", h.openAPISpec)

	{
		svcs := service.New(h.Repos, h.notifier())
		svcs.Schema = h.fieldSchema
		apiHandler := handler.New(svcs)
		registerPolicyRoutes(mux, apiHandler)
		registerPolicyGroupRoutes(mux, apiHandler)
		registerExceptionRoutes(mux, apiHandler)
		registerSchemaAndSummaryRoutes(mux, apiHandler)
		registerAdmissionRoutes(mux, apiHandler)
		registerClusterRoutes(mux, apiHandler)
		registerDistributionRoutes(mux, apiHandler)
		registerPlatformRoutes(mux, apiHandler)
	}
	return &Server{Handler: rejectNonCanonicalAPIPaths(apiFallback(mux)), mux: mux}, nil
}

func apiFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" || !strings.HasPrefix(r.URL.Path, "/api/") {
			mux.ServeHTTP(w, r)
			return
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		if rec.Code == http.StatusNotFound {
			writeErr(w, http.StatusNotFound, "unknown API route")
			return
		}
		if rec.Code == http.StatusMethodNotAllowed {
			methods := strings.Split(rec.Header().Get("Allow"), ", ")
			allowed := methods[:0]
			for _, method := range methods {
				if method != http.MethodHead {
					allowed = append(allowed, method)
				}
			}
			w.Header().Set("Allow", strings.Join(allowed, ","))
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		for key, values := range rec.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	})
}

func rejectNonCanonicalAPIPaths(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escaped := r.URL.EscapedPath()
		cleaned := path.Clean(escaped)
		if strings.HasSuffix(escaped, "/") && cleaned != "/" {
			cleaned += "/"
		}
		isAPIPath := func(value string) bool {
			return value == "/api/v1" || strings.HasPrefix(value, "/api/v1/")
		}
		if (isAPIPath(escaped) || isAPIPath(cleaned) || r.URL.Path == "/api/v1") && (cleaned != escaped || r.URL.Path == "/api/v1") {
			writeErr(w, http.StatusNotFound, "unknown API route")
			return
		}
		next.ServeHTTP(w, r)
	})
}
