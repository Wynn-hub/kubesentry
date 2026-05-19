package webhook

import (
	"net/http"
)

// ServerConfig holds TLS and address configuration.
type ServerConfig struct {
	Addr     string
	CertFile string
	KeyFile  string
}

// Server is an HTTPS server serving the webhook endpoints.
type Server struct {
	Handler *http.ServeMux
	config  ServerConfig
}

func NewServer(store PolicyStore, exceptions ExceptionStore, cfg ServerConfig) *Server {
	if exceptions == nil {
		exceptions = noExemptions{}
	}
	mux := http.NewServeMux()
	s := &Server{Handler: mux, config: cfg}

	mux.Handle("/validate", NewHandlerWithExceptions(store, exceptions))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if store.IsReady() && exceptions.IsReady() {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})

	return s
}

func (s *Server) ListenAndServeTLS() error {
	addr := s.config.Addr
	if addr == "" {
		addr = ":8443"
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: s.Handler,
	}
	return srv.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
}
