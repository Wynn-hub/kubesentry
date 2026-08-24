package handler

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func (h *Handler) GetFieldSchema(ctx context.Context, p dto.GetFieldSchemaParams) (dto.GetFieldSchemaRes, error) {
	if h.svcs.Schema == nil {
		return &dto.GetFieldSchemaBadGateway{Code: 50200, Message: "schema discovery is not configured"}, nil
	}
	f, e := h.svcs.Schema(ctx, p)
	if e != nil {
		return &dto.GetFieldSchemaBadGateway{Code: 50200, Message: e.Error()}, nil
	}
	return &dto.FieldSchemaResponse{Code: 0, Message: "success", Data: f}, nil
}
