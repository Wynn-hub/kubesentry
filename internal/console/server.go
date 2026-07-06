package console

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Handlers holds dependencies shared by all API handlers.
type Handlers struct {
	Client client.Client
}

// Register wires all /api/v1 routes. Later tasks append routes here.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/policies", h.listPolicies)
	mux.HandleFunc("GET /api/v1/policies/{name}", h.getPolicy)
	mux.HandleFunc("POST /api/v1/policies", h.createPolicy)
	mux.HandleFunc("PUT /api/v1/policies/{name}", h.updatePolicy)
	mux.HandleFunc("POST /api/v1/policies/validate", h.validatePolicy)
	mux.HandleFunc("DELETE /api/v1/policies/{name}", h.deletePolicy)
	mux.HandleFunc("GET /api/v1/policies/{name}/versions", h.listPolicyVersions)
	mux.HandleFunc("POST /api/v1/policies/{name}/rollback", h.rollbackPolicy)
}

// Server is the console HTTP server (plain HTTP; accessed via port-forward).
type Server struct {
	Handler http.Handler
}

func NewServer(h *Handlers, dist fs.FS, ready func() bool) *Server {
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

	mux.Handle("/", spaHandler(dist))
	return &Server{Handler: mux}
}

// spaHandler serves the embedded SPA: real files as-is, unknown non-API
// paths fall back to index.html (history routing), unknown API paths 404.
func spaHandler(dist fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "unknown API route")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" && name != "index.html" {
			if f, err := dist.Open(name); err == nil {
				_ = f.Close()
				http.FileServerFS(dist).ServeHTTP(w, r)
				return
			}
		}
		idx, err := dist.Open("index.html")
		if err != nil {
			http.Error(w, "console UI not built", http.StatusServiceUnavailable)
			return
		}
		defer idx.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, idx)
	})
}
