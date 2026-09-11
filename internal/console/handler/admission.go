package handler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) ListAdmissionEvents(ctx context.Context, params dto.ListAdmissionEventsParams) (dto.ListAdmissionEventsRes, error) {
	data, err := h.svcs.Admission.List(ctx, admissionListOptions(params))
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.ListAdmissionEventsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.ListAdmissionEventsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.AdmissionEventListData](data)
	if err != nil {
		return nil, err
	}
	return &dto.AdmissionEventListResponse{Code: 0, Message: "success", Data: dto.NewOptAdmissionEventListData(v)}, nil
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

func (h *Handler) GetAdmissionEvent(ctx context.Context, params dto.GetAdmissionEventParams) (dto.GetAdmissionEventRes, error) {
	data, err := h.svcs.Admission.Get(ctx, string(params.ID))
	if err != nil {
		var validationErr *service.ValidationError
		switch {
		case errors.As(err, &validationErr):
			return &dto.GetAdmissionEventBadRequest{Code: 40000, Message: err.Error()}, nil
		case errors.Is(err, service.ErrNotFound):
			return &dto.GetAdmissionEventNotFound{Code: 40400, Message: "admission event not found"}, nil
		default:
			return &dto.GetAdmissionEventInternalServerError{Code: 50000, Message: "request failed"}, nil
		}
	}
	v, err := dtoData[dto.AdmissionEventDetail](data)
	if err != nil {
		return nil, err
	}
	return &dto.AdmissionEventDetailResponse{Code: 0, Message: "success", Data: dto.NewOptAdmissionEventDetail(v)}, nil
}

func (h *Handler) GetAdmissionEventTrend(ctx context.Context, params dto.GetAdmissionEventTrendParams) (dto.GetAdmissionEventTrendRes, error) {
	data, err := h.svcs.Admission.Trend(ctx, params.From, params.To)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.GetAdmissionEventTrendBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.GetAdmissionEventTrendInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[[]dto.AdmissionEventTrendItem](data)
	if err != nil {
		return nil, err
	}
	return &dto.AdmissionEventTrendResponse{Code: 0, Message: "success", Data: v}, nil
}

func (h *Handler) GetAdmissionEventSettings(ctx context.Context) (dto.GetAdmissionEventSettingsRes, error) {
	data, err := h.svcs.Admission.Settings(ctx)
	if err != nil {
		return &dto.GetAdmissionEventSettingsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.AdmissionEventSettings](data)
	if err != nil {
		return nil, err
	}
	return &dto.AdmissionEventSettingsResponse{Code: 0, Message: "success", Data: dto.NewOptAdmissionEventSettings(v)}, nil
}

func (h *Handler) UpdateAdmissionEventSettings(ctx context.Context, req *dto.AdmissionEventSettingsRequest) (dto.UpdateAdmissionEventSettingsRes, error) {
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var input dto.AdmissionSettingsInput
	if err := json.Unmarshal(encoded, &input); err != nil {
		return &dto.UpdateAdmissionEventSettingsBadRequest{Code: 40000, Message: err.Error()}, nil
	}
	data, err := h.svcs.Admission.UpdateSettings(ctx, &input)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.UpdateAdmissionEventSettingsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.UpdateAdmissionEventSettingsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.AdmissionEventSettings](data)
	if err != nil {
		return nil, err
	}
	return &dto.AdmissionEventSettingsResponse{Code: 0, Message: "success", Data: dto.NewOptAdmissionEventSettings(v)}, nil
}
