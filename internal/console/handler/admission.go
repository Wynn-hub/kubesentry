package handler

import (
	"context"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) ListAdmissionEvents(ctx context.Context, params dto.ListAdmissionEventsParams) (dto.ListAdmissionEventsRes, error) {
	data, err := h.svcs.Admission.List(ctx, params)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.ListAdmissionEventsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.ListAdmissionEventsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.AdmissionEventListResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventListData{Set: true, Value: *data}}, nil
}

func (h *Handler) GetAdmissionEvent(ctx context.Context, params dto.GetAdmissionEventParams) (dto.GetAdmissionEventRes, error) {
	data, err := h.svcs.Admission.Get(ctx, params.ID)
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
	return &dto.AdmissionEventDetailResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventDetail{Set: true, Value: *data}}, nil
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
	return &dto.AdmissionEventTrendResponse{Code: 0, Message: "success", Data: data}, nil
}

func (h *Handler) GetAdmissionEventSettings(ctx context.Context) (dto.GetAdmissionEventSettingsRes, error) {
	data, err := h.svcs.Admission.Settings(ctx)
	if err != nil {
		return &dto.GetAdmissionEventSettingsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.AdmissionEventSettingsResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventSettings{Set: true, Value: *data}}, nil
}

func (h *Handler) UpdateAdmissionEventSettings(ctx context.Context, req *dto.AdmissionEventSettingsRequest) (dto.UpdateAdmissionEventSettingsRes, error) {
	data, err := h.svcs.Admission.UpdateSettings(ctx, req)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return &dto.UpdateAdmissionEventSettingsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.UpdateAdmissionEventSettingsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.AdmissionEventSettingsResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventSettings{Set: true, Value: *data}}, nil
}
