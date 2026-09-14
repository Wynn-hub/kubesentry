package handler

import (
	"context"
	"errors"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) GetPlatformOverview(ctx context.Context, p dto.GetPlatformOverviewParams) (dto.GetPlatformOverviewRes, error) {
	data, err := h.svcs.Platform.Overview(ctx, platformOverviewOptions(p))
	if err != nil {
		var validation *service.ValidationError
		if errors.As(err, &validation) {
			return &dto.GetPlatformOverviewBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.GetPlatformOverviewInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.PlatformOverview](data)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformOverviewResponse{Code: 0, Message: "success", Data: v}, nil
}

func platformOverviewOptions(p dto.GetPlatformOverviewParams) dto.PlatformOverviewOptions {
	health := ""
	if value, ok := p.Health.Get(); ok {
		health = string(value)
	}
	return dto.PlatformOverviewOptions{Page: p.Page.Or(1), PageSize: int(p.PageSize.Or(dto.PageSize20)), Health: health}
}

func (h *Handler) GetPlatformInstanceTrend(ctx context.Context, p dto.GetPlatformInstanceTrendParams) (dto.GetPlatformInstanceTrendRes, error) {
	data, err := h.svcs.Platform.InstanceTrend(ctx, platformTrendOptions(p))
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
	v, err := dtoData[dto.PlatformTrend](data)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformTrendResponse{Code: 0, Message: "success", Data: v}, nil
}

func platformTrendOptions(p dto.GetPlatformInstanceTrendParams) dto.PlatformTrendOptions {
	return dto.PlatformTrendOptions{Component: string(p.Component), Instance: p.Instance, Cluster: p.Cluster.Or(""), From: optTime(p.From), To: optTime(p.To)}
}

func (h *Handler) ListPlatformLeadershipEvents(ctx context.Context, p dto.ListPlatformLeadershipEventsParams) (dto.ListPlatformLeadershipEventsRes, error) {
	data, err := h.svcs.Platform.LeadershipEvents(ctx, platformLeadershipOptions(p))
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
	v, err := dtoData[dto.LeadershipEventListResponseData](data)
	if err != nil {
		return nil, err
	}
	return &dto.LeadershipEventListResponse{Code: 0, Message: "success", Data: v}, nil
}

func platformLeadershipOptions(p dto.ListPlatformLeadershipEventsParams) dto.PlatformLeadershipOptions {
	component := ""
	if value, ok := p.Component.Get(); ok {
		component = string(value)
	}
	return dto.PlatformLeadershipOptions{Page: p.Page.Or(1), PageSize: int(p.PageSize.Or(dto.PageSize20)), Component: component, Cluster: p.Cluster.Or(""), From: optTime(p.From), To: optTime(p.To)}
}

func (h *Handler) GetPlatformSettings(ctx context.Context) (dto.GetPlatformSettingsRes, error) {
	data, err := h.svcs.Platform.Settings(ctx)
	if err != nil {
		return &dto.ErrorResponse{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.PlatformSettings](data)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformSettingsResponse{Code: 0, Message: "success", Data: v}, nil
}

func (h *Handler) UpdatePlatformSettings(ctx context.Context, req *dto.PlatformSettingsRequest) (dto.UpdatePlatformSettingsRes, error) {
	data, err := h.svcs.Platform.UpdateSettings(ctx, dto.PlatformSettingsInput{Enabled: req.Enabled, RetentionDays: req.RetentionDays})
	if err != nil {
		var validation *service.ValidationError
		if errors.As(err, &validation) {
			return &dto.UpdatePlatformSettingsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.UpdatePlatformSettingsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.PlatformSettings](data)
	if err != nil {
		return nil, err
	}
	return &dto.PlatformSettingsResponse{Code: 0, Message: "success", Data: v}, nil
}

func optTime(value dto.OptDateTime) *time.Time {
	if result, ok := value.Get(); ok {
		return &result
	}
	return nil
}
