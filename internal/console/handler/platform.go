package handler

import (
	"context"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) GetPlatformOverview(ctx context.Context, p dto.GetPlatformOverviewParams) (dto.GetPlatformOverviewRes, error) {
	data, err := h.svcs.Platform.Overview(ctx, p)
	if err != nil {
		var validation *service.ValidationError
		if errors.As(err, &validation) {
			return &dto.GetPlatformOverviewBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.GetPlatformOverviewInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PlatformOverviewResponse{Code: 0, Message: "success", Data: data}, nil
}

func (h *Handler) GetPlatformInstanceTrend(ctx context.Context, p dto.GetPlatformInstanceTrendParams) (dto.GetPlatformInstanceTrendRes, error) {
	data, err := h.svcs.Platform.InstanceTrend(ctx, p)
	if err != nil {
		var validation *service.ValidationError
		switch {
		case errors.As(err, &validation):
			return &dto.GetPlatformInstanceTrendBadRequest{Code: 40000, Message: err.Error()}, nil
		case errors.Is(err, service.ErrNotFound):
			return &dto.GetPlatformInstanceTrendNotFound{Code: 40400, Message: "cluster not found"}, nil
		default:
			return &dto.GetPlatformInstanceTrendInternalServerError{Code: 50000, Message: "request failed"}, nil
		}
	}
	return &dto.PlatformTrendResponse{Code: 0, Message: "success", Data: data}, nil
}

func (h *Handler) ListPlatformLeadershipEvents(ctx context.Context, p dto.ListPlatformLeadershipEventsParams) (dto.ListPlatformLeadershipEventsRes, error) {
	data, err := h.svcs.Platform.LeadershipEvents(ctx, p)
	if err != nil {
		var validation *service.ValidationError
		switch {
		case errors.As(err, &validation):
			return &dto.ListPlatformLeadershipEventsBadRequest{Code: 40000, Message: err.Error()}, nil
		case errors.Is(err, service.ErrNotFound):
			return &dto.ListPlatformLeadershipEventsNotFound{Code: 40400, Message: "cluster not found"}, nil
		default:
			return &dto.ListPlatformLeadershipEventsInternalServerError{Code: 50000, Message: "request failed"}, nil
		}
	}
	return &dto.LeadershipEventListResponse{Code: 0, Message: "success", Data: data}, nil
}

func (h *Handler) GetPlatformSettings(ctx context.Context) (dto.GetPlatformSettingsRes, error) {
	data, err := h.svcs.Platform.Settings(ctx)
	if err != nil {
		return &dto.ErrorResponse{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PlatformSettingsResponse{Code: 0, Message: "success", Data: data}, nil
}

func (h *Handler) UpdatePlatformSettings(ctx context.Context, req *dto.PlatformSettingsRequest) (dto.UpdatePlatformSettingsRes, error) {
	data, err := h.svcs.Platform.UpdateSettings(ctx, req)
	if err != nil {
		var validation *service.ValidationError
		if errors.As(err, &validation) {
			return &dto.UpdatePlatformSettingsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.UpdatePlatformSettingsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PlatformSettingsResponse{Code: 0, Message: "success", Data: data}, nil
}
