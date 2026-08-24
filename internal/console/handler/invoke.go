package handler

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/go-faster/jx"
)

func (h *Handler) invokeObject(ctx context.Context, method, path string, body []byte) (map[string]jx.Raw, int, error) {
	return h.platformInvoke(ctx, method, path, body)
}

func objectError(status int) *dto.ErrorResponse {
	if status < 400 {
		status = 500
	}
	return &dto.ErrorResponse{Code: status * 100, Message: "request failed"}
}
