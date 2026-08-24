package console

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/handler"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// Handlers holds dependencies shared by all API handlers.
type Handlers struct {
	// Repos is the GORM data-access layer backing the policy/group/exception/
	// summary handlers (Task 9 + Task 10). Discovery is still used by
	// handlers_schema.go, the one remaining handler that legitimately talks
	// to a live cluster (OpenAPI field schema for the visual policy builder)
	// rather than the database — out of scope for this task.
	Repos         gateway.Repos
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

// RegisterLegacy wires routes that have not yet moved to ogen.
func (h *Handlers) RegisterLegacy(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/summary", h.summary)
	mux.HandleFunc("GET /api/v1/admission-events", h.listAdmissionEvents)
	mux.HandleFunc("GET /api/v1/admission-events/{id}", h.getAdmissionEvent)
	mux.HandleFunc("GET /api/v1/admission-events/trend", h.admissionEventTrend)
	mux.HandleFunc("GET /api/v1/admission-event-settings", h.getAdmissionEventSettings)
	mux.HandleFunc("PUT /api/v1/admission-event-settings", h.updateAdmissionEventSettings)
	mux.HandleFunc("GET /api/v1/admission-link-incidents", h.listAdmissionLinkIncidents)
	mux.HandleFunc("GET /api/v1/admission-link-incidents/{id}", h.getAdmissionLinkIncident)
	mux.HandleFunc("GET /api/v1/platform/overview", h.platformOverview)
	mux.HandleFunc("GET /api/v1/platform/instances/{component}/{instance}/trend", h.platformInstanceTrend)
	mux.HandleFunc("GET /api/v1/platform/leadership-events", h.platformLeadershipEvents)
	mux.HandleFunc("GET /api/v1/platform/settings", h.getPlatformSettings)
	mux.HandleFunc("PUT /api/v1/platform/settings", h.updatePlatformSettings)
	mux.HandleFunc("GET /api/v1/schema/fields", h.getFieldSchema)
	mux.HandleFunc("GET /api/v1/clusters", h.listClusters)
	mux.HandleFunc("POST /api/v1/clusters", h.createCluster)
	mux.HandleFunc("GET /api/v1/clusters/{name}", h.getCluster)
	mux.HandleFunc("PUT /api/v1/clusters/{name}", h.updateClusterLabels)
	mux.HandleFunc("DELETE /api/v1/clusters/{name}", h.deleteCluster)
	mux.HandleFunc("POST /api/v1/clusters/{name}/reissue-token", h.reissueClusterToken)
	mux.HandleFunc("GET /api/v1/clusters/{name}/latest-install-params", h.getLatestInstallParams)
	mux.HandleFunc("POST /api/v1/clusters/{name}/uninstall", h.createUninstallTask)
	mux.HandleFunc("GET /api/v1/clusters/{name}/uninstall-tasks", h.listUninstallTasks)
	mux.HandleFunc("POST /api/v1/clusters/{name}/uninstall-tasks/{taskID}/retry", h.retryUninstallTask)
	mux.HandleFunc("GET /api/v1/install-records", h.listInstallRecords)
	mux.HandleFunc("GET /api/v1/distributions", h.listDistributions)
	mux.HandleFunc("POST /api/v1/distributions", h.createDistribution)
	mux.HandleFunc("GET /api/v1/distributions/{name}", h.getDistribution)
	mux.HandleFunc("PUT /api/v1/distributions/{name}", h.updateDistribution)
	mux.HandleFunc("DELETE /api/v1/distributions/{name}", h.deleteDistribution)
}

// Server is the console HTTP server (plain HTTP; accessed via port-forward,
// or fronted by the nginx `web` container which proxies /api/* and /healthz
// here and serves the built SPA itself — see charts/kubesentry-gateway/console).
type Server struct {
	Handler http.Handler
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

	if h.Repos.Policies != nil {
		svcs := service.New(repository.FromGatewayRepos(h.Repos), h.notifier())
		ogenServer, err := dto.NewServer(
			handler.New(svcs),
			dto.WithPathPrefix("/api/v1"),
			dto.WithMiddleware(TraceIDMiddleware()),
		)
		if err != nil {
			return nil, fmt.Errorf("construct policy ogen server: %w", err)
		}
		mux.Handle("/api/v1/policies", ogenServer)
		mux.Handle("/api/v1/policies/", ogenServer)
		mux.Handle("/api/v1/policygroups", ogenServer)
		mux.Handle("/api/v1/policygroups/", ogenServer)
		mux.Handle("/api/v1/exceptions", ogenServer)
		mux.Handle("/api/v1/exceptions/", ogenServer)
	}
	h.RegisterLegacy(mux)

	// The nginx `web` container only proxies /healthz and /api/* here (see
	// charts/kubesentry-gateway/templates/console/nginx-configmap.yaml); anything else served by
	// this process directly (e.g. port-forward during dev) is an unmatched
	// API route, so keep the same JSON error envelope other handlers use
	// rather than falling back to ServeMux's plain-text 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "unknown API route")
			return
		}
		http.NotFound(w, r)
	})

	return &Server{Handler: mux}, nil
}
