package handler

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

// @ID getSummary
// @Summary 获取平台资源摘要
// @Description 查询策略、策略组、例外和集群的汇总数量。
// @Tags Summary
// @Produce json
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SummaryView}
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /summary [get]
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
