package console

import (
	"net/http"
	"strings"
	"sync"

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

// Register wires all /api/v1 routes. Later tasks append routes here.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/policies", h.listPolicies)
	mux.HandleFunc("GET /api/v1/policies/resource-suggestions", h.resourceSuggestions)
	mux.HandleFunc("GET /api/v1/policies/{name}", h.getPolicy)
	mux.HandleFunc("POST /api/v1/policies", h.createPolicy)
	mux.HandleFunc("PUT /api/v1/policies/{name}", h.updatePolicy)
	mux.HandleFunc("POST /api/v1/policies/validate", h.validatePolicy)
	mux.HandleFunc("DELETE /api/v1/policies/{name}", h.deletePolicy)
	mux.HandleFunc("GET /api/v1/policies/{name}/versions", h.listPolicyVersions)
	mux.HandleFunc("POST /api/v1/policies/{name}/rollback", h.rollbackPolicy)
	mux.HandleFunc("GET /api/v1/policygroups", h.listGroups)
	mux.HandleFunc("POST /api/v1/policygroups", h.createGroup)
	mux.HandleFunc("GET /api/v1/policygroups/{name}", h.getGroup)
	mux.HandleFunc("PUT /api/v1/policygroups/{name}", h.updateGroup)
	mux.HandleFunc("PUT /api/v1/policygroups/{name}/enabled", h.setGroupEnabled)
	mux.HandleFunc("DELETE /api/v1/policygroups/{name}", h.deleteGroup)
	mux.HandleFunc("GET /api/v1/exceptions", h.listExceptions)
	mux.HandleFunc("POST /api/v1/exceptions", h.createException)
	mux.HandleFunc("GET /api/v1/exceptions/{name}", h.getException)
	mux.HandleFunc("PUT /api/v1/exceptions/{name}", h.updateException)
	mux.HandleFunc("DELETE /api/v1/exceptions/{name}", h.deleteException)
	mux.HandleFunc("GET /api/v1/summary", h.summary)
	mux.HandleFunc("GET /api/v1/admission-events", h.listAdmissionEvents)
	mux.HandleFunc("GET /api/v1/admission-events/{id}", h.getAdmissionEvent)
	mux.HandleFunc("GET /api/v1/admission-events/trend", h.admissionEventTrend)
	mux.HandleFunc("GET /api/v1/admission-event-settings", h.getAdmissionEventSettings)
	mux.HandleFunc("PUT /api/v1/admission-event-settings", h.updateAdmissionEventSettings)
	mux.HandleFunc("GET /api/v1/observability/admission/overview", h.admissionObservationOverview)
	mux.HandleFunc("GET /api/v1/observability/admission/trend", h.admissionObservationTrend)
	mux.HandleFunc("GET /api/v1/observability/admission/clusters", h.admissionObservationClusters)
	mux.HandleFunc("GET /api/v1/observability/settings", h.getObservationSettings)
	mux.HandleFunc("PUT /api/v1/observability/settings", h.updateObservationSettings)
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
func NewServer(h *Handlers, ready func() bool) *Server {
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

	h.Register(mux)

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

	return &Server{Handler: mux}
}
