// Package handler implements the ogen-generated ServerInterface. Each method
// is protocol-only: parse the request, call a service, map the result onto a
// dto. No business logic and no repository access live here.
package handler

import (
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// Handler implements the generated dto.Handler interface. Methods are added
// per resource domain as batches migrate.
type Handler struct {
	dto.UnimplementedHandler
	svcs service.Services
}

func New(svcs service.Services) *Handler {
	return &Handler{svcs: svcs}
}
