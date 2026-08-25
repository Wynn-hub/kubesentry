package console

import (
	_ "embed"
	"net/http"
)

// bundledOpenAPISpec is generated from api/console/console-api.yaml and its
// schema fragments. Keeping the resolved document alongside this package makes
// the API documentation endpoint independent of the repository layout and any
// runtime volume mounts in the console-api container.
//
//go:embed openapi_bundle.json
var bundledOpenAPISpec []byte

func (h *Handlers) openAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bundledOpenAPISpec)
}
