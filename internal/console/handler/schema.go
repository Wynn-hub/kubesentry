package handler

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func (h *Handler) GetFieldSchema(ctx context.Context, p dto.GetFieldSchemaParams) (dto.GetFieldSchemaRes, error) {
	if h.svcs.Schema == nil {
		return &dto.GetFieldSchemaBadGateway{Code: 50200, Message: "schema discovery is not configured"}, nil
	}
	f, e := h.svcs.Schema(ctx, fieldSchemaOptions(p))
	if e != nil {
		return &dto.GetFieldSchemaBadGateway{Code: 50200, Message: e.Error()}, nil
	}
	data, err := dtoData[[]dto.FieldNode](f)
	if err != nil {
		return nil, err
	}
	return &dto.FieldSchemaResponse{Code: 0, Message: "success", Data: data}, nil
}

func fieldSchemaOptions(p dto.GetFieldSchemaParams) dto.FieldSchemaOptions {
	return dto.FieldSchemaOptions{Group: p.Group.Or(""), Version: p.Version, Resource: p.Resource, Refresh: p.Refresh.Or(false)}
}
