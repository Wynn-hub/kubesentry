package handler

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func (h *Handler) GetSummary(ctx context.Context) (dto.GetSummaryRes, error) {
	d, e := h.svcs.Summary.Get(ctx)
	if e != nil {
		return &dto.ErrorResponse{Code: 50000, Message: e.Error()}, nil
	}
	v, err := dtoData[dto.SummaryData](d)
	if err != nil {
		return nil, err
	}
	return &dto.SummaryResponse{Code: 0, Message: "success", Data: dto.NewOptSummaryData(v)}, nil
}
