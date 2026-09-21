package handler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// @ID listAdmissionEvents
// @Summary 分页查询准入事件
// @Description 按集群、判决、策略、命名空间和资源条件分页检索准入事件。
// @Tags AdmissionEvent
// @Produce json
// @Param page query int false "页码，从 1 开始。省略时为 1" minimum(1) default(1)
// @Param pageSize query int false "每页记录数。仅接受 10、20、50、100。省略时为 20" Enums(10,20,50,100) default(20)
// @Param sortBy query string false "排序字段" Enums(occurredAt,createdAt,verdict) default(occurredAt)
// @Param sortOrder query string false "排序方向" Enums(asc,desc) default(desc)
// @Param cluster query []string false "集群名称，逗号分隔" collectionFormat(csv)
// @Param verdict query []string false "准入结论，逗号分隔多值，取 OR" collectionFormat(csv) Enums(audit,deny)
// @Param policy query string false "策略名称"
// @Param namespace query []string false "命名空间，逗号分隔多值" collectionFormat(csv)
// @Param gvk query []string false "资源名称或 group/resource，逗号分隔" collectionFormat(csv)
// @Param resourceName query string false "资源名称"
// @Param username query string false "请求用户"
// @Param from query string false "查询起始时间" format(date-time)
// @Param to query string false "查询结束时间" format(date-time)
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.AdmissionEventPageView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /admission-events [get]
func (h *Handler) ListAdmissionEvents(ctx context.Context, params dto.ListAdmissionEventsParams) (dto.ListAdmissionEventsRes, error) {
	data, err := h.svcs.Admission.List(ctx, admissionListOptions(params))
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.AdmissionEventBadRequestHTTPError{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.AdmissionEventInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.AdmissionEventListHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}

func admissionListOptions(params dto.ListAdmissionEventsParams) dto.AdmissionEventListOptions {
	verdicts := make([]string, len(params.Verdict))
	for i := range params.Verdict {
		verdicts[i] = string(params.Verdict[i])
	}
	var from, to *time.Time
	if value, ok := params.From.Get(); ok {
		from = &value
	}
	if value, ok := params.To.Get(); ok {
		to = &value
	}
	return dto.AdmissionEventListOptions{
		Page: params.Page.Or(1), PageSize: int(params.PageSize.Or(dto.PageSize20)), SortBy: string(params.SortBy.Or(dto.ListAdmissionEventsSortByOccurredAt)), SortOrder: string(params.SortOrder.Or(dto.ListAdmissionEventsSortOrderDesc)),
		Clusters: params.Cluster, Verdicts: verdicts, PolicyName: params.Policy.Or(""), Namespaces: params.Namespace, GVRs: params.Gvk, ResourceName: params.ResourceName.Or(""), Username: params.Username.Or(""), From: from, To: to,
	}
}

// @ID getAdmissionEvent
// @Summary 查询准入事件详情
// @Description 查询准入事件及其原始 metadata/spec 快照。
// @Tags AdmissionEvent
// @Produce json
// @Param id path dto.AdmissionEventPath true "准入事件数据库 ID"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.AdmissionEventDetailView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /admission-events/{id} [get]
func (h *Handler) GetAdmissionEvent(ctx context.Context, params dto.GetAdmissionEventParams) (dto.GetAdmissionEventRes, error) {
	data, err := h.svcs.Admission.Get(ctx, string(params.ID))
	if err != nil {
		var validationErr *service.ValidationError
		switch {
		case errors.As(err, &validationErr):
			return &dto.AdmissionEventBadRequestHTTPError{Code: 40000, Message: err.Error()}, nil
		case errors.Is(err, service.ErrNotFound):
			return &dto.AdmissionEventNotFoundHTTPError{Code: 40400, Message: "admission event not found"}, nil
		default:
			return &dto.AdmissionEventInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
		}
	}
	return &dto.AdmissionEventDetailHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}

// @ID getAdmissionEventTrend
// @Summary 查询准入事件趋势
// @Description 按时间范围返回准入事件的 audit/deny 趋势数据。
// @Tags AdmissionEvent
// @Produce json
// @Param from query string true "查询起始时间" format(date-time)
// @Param to query string true "查询结束时间" format(date-time)
// @Success 200 {object} dto.SwagOptionalResponse{data=[]dto.AdmissionTrendItemView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /admission-events/trend [get]
func (h *Handler) GetAdmissionEventTrend(ctx context.Context, params dto.GetAdmissionEventTrendParams) (dto.GetAdmissionEventTrendRes, error) {
	data, err := h.svcs.Admission.Trend(ctx, params.From, params.To)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.AdmissionEventBadRequestHTTPError{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.AdmissionEventInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.AdmissionEventTrendHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}

// @ID getAdmissionEventSettings
// @Summary 获取准入事件设置
// @Description 查询准入事件留存、重试、队列和存储策略配置。
// @Tags AdmissionEvent
// @Produce json
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.AdmissionSettingsView}
// @Failure 500 {object} dto.SwagErrorResponse
// @Failure 503 {object} dto.SwagErrorResponse
// @Router /admission-event-settings [get]
func (h *Handler) GetAdmissionEventSettings(ctx context.Context) (dto.GetAdmissionEventSettingsRes, error) {
	data, err := h.svcs.Admission.Settings(ctx)
	if err != nil {
		return &dto.AdmissionEventInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.AdmissionEventSettingsHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}

// @ID updateAdmissionEventSettings
// @Summary 更新准入事件设置
// @Description 更新准入事件留存、重试、队列和存储策略配置。
// @Tags AdmissionEvent
// @Accept json
// @Produce json
// @Param body body dto.AdmissionSettingsInput true "准入事件设置"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.AdmissionSettingsView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /admission-event-settings [put]
func (h *Handler) UpdateAdmissionEventSettings(ctx context.Context, req *dto.AdmissionEventSettingsRequest) (dto.UpdateAdmissionEventSettingsRes, error) {
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var input dto.AdmissionSettingsInput
	if err := json.Unmarshal(encoded, &input); err != nil {
		return &dto.AdmissionEventBadRequestHTTPError{Code: 40000, Message: err.Error()}, nil
	}
	data, err := h.svcs.Admission.UpdateSettings(ctx, &input)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.AdmissionEventBadRequestHTTPError{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.AdmissionEventInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.AdmissionEventSettingsHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}
