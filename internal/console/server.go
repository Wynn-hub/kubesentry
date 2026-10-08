package console

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
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
	Gateway                 gatewaypb.HubClient
	SessionAbsoluteDuration time.Duration
	SessionIdleDuration     time.Duration
	SessionCookieSecure     bool

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

type routeRegistrar interface {
	HandleFunc(string, http.HandlerFunc)
}

type routeGroup struct {
	mux        *http.ServeMux
	middleware func(http.Handler) http.Handler
}

func (g routeGroup) HandleFunc(pattern string, handler http.HandlerFunc) {
	if g.middleware == nil {
		g.mux.HandleFunc(pattern, handler)
		return
	}
	g.mux.Handle(pattern, g.middleware(handler))
}

// NewServer wires the REST API only; the SPA is served by a separate nginx
// container (see Dockerfile.console-web and the gateway chart's nginx ConfigMap).
// ready reports whether console-api's dependencies (currently: the database)
// are reachable; nil always reports ready.
func NewServer(h *Handlers, ready func() bool) (*Server, error) {
	return newServer(h, ready, nil)
}

// newServer keeps operational, public, and protected routes distinct. A nil
// protectedMiddleware preserves today's anonymous access; a future auth plan
// can supply one here without changing individual route registrations.
func newServer(h *Handlers, ready func() bool, protectedMiddleware func(http.Handler) http.Handler) (*Server, error) {
	mux := http.NewServeMux()
	svcs := service.New(h.Repos, h.notifier())
	svcs.Auth.SetDurations(h.SessionAbsoluteDuration, h.SessionIdleDuration)
	var auth *authHTTP
	if h.Repos.Identity != nil {
		auth = newAuthHTTP(svcs.Auth, h.SessionAbsoluteDuration, h.SessionCookieSecure)
		if protectedMiddleware == nil {
			protectedMiddleware = auth.middleware
		}
	}
	operational := routeGroup{mux: mux}
	public := routeGroup{mux: mux}
	protected := routeGroup{mux: mux, middleware: protectedMiddleware}

	operational.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	operational.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ready == nil || ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})
	public.HandleFunc("GET /api/docs/openapi.json", h.openAPISpec)
	if auth != nil {
		public.HandleFunc("GET /api/v1/setup/status", auth.setupStatus)
		public.HandleFunc("POST /api/v1/setup/admin", auth.setupAdmin)
		public.HandleFunc("POST /api/v1/auth/login", auth.login)
		public.HandleFunc("POST /api/v1/auth/activate", auth.activate)
		protected.HandleFunc("POST /api/v1/auth/logout", auth.logout)
		protected.HandleFunc("POST /api/v1/auth/change-password", auth.changePassword)
		protected.HandleFunc("GET /api/v1/me", auth.me)
		identity := identityHTTP{service: svcs.Identity}
		protected.HandleFunc("GET /api/v1/tenants", identity.listTenants)
		protected.HandleFunc("POST /api/v1/tenants", identity.createTenant)
		protected.HandleFunc("PUT /api/v1/tenants/{id}/status", identity.setTenantStatus)
		protected.HandleFunc("GET /api/v1/users", identity.listUsers)
		protected.HandleFunc("POST /api/v1/users", identity.createUser)
		protected.HandleFunc("PUT /api/v1/users/{id}/status", identity.setUserStatus)
		protected.HandleFunc("POST /api/v1/users/{id}/reset-password", identity.resetPassword)
		protected.HandleFunc("DELETE /api/v1/users/{id}", identity.deleteUser)
		protected.HandleFunc("PUT /api/v1/clusters/{name}/tenant", func(w http.ResponseWriter, r *http.Request) {
			var input dto.ClusterTenantRequest
			if !decodeAuthJSON(w, r, &input) {
				return
			}
			err := svcs.Cluster.AssignTenant(requestContext(r), r.PathValue("name"), input.TenantID)
			switch {
			case errors.Is(err, service.ErrNotFound):
				writeErr(w, http.StatusNotFound, "not found")
			case service.IsConflict(err):
				writeErr(w, http.StatusConflict, err.Error())
			case err != nil:
				writeErr(w, http.StatusInternalServerError, "request failed")
			default:
				writeJSON(w, http.StatusOK, Envelope{Code: 0, Message: "success"})
			}
		})
	}

	{
		svcs.Schema = h.fieldSchema
		apiHandler := handler.New(svcs)
		registerPolicyRoutes(protected, apiHandler)
		registerPolicyGroupRoutes(protected, apiHandler)
		registerExceptionRoutes(protected, apiHandler)
		registerSchemaAndSummaryRoutes(protected, apiHandler)
		registerAdmissionRoutes(protected, apiHandler)
		registerClusterRoutes(protected, apiHandler)
		registerDistributionRoutes(protected, apiHandler)
		registerPlatformRoutes(protected, apiHandler)
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
