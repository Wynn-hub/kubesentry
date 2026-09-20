// Package handler maps the Console HTTP contract onto services. No business
// logic and no repository access live here.
package handler

import (
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

type Handler struct {
	svcs service.Services
}

func New(svcs service.Services) *Handler {
	return &Handler{svcs: svcs}
}
