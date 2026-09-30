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
// @Security SessionCookie
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SummaryView}
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /summary [get]
func (h *Handler) GetSummary(ctx context.Context) (dto.GetSummaryRes, error) {
	d, e := h.svcs.Summary.Get(ctx)
	if e != nil {
		return &dto.SummaryHTTPError{Code: 50000, Message: e.Error()}, nil
	}
	return &dto.SummaryHTTPResponse{Code: 0, Message: "success", Data: d}, nil
}
