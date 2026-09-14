package handler

import (
	"context"
	"errors"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// @ID getPlatformOverview
// @Summary 获取平台运行概览
// @Description 查询控制面和成员集群的组件健康、实例和资源摘要。
// @Tags Platform
// @Produce json
// @Param page query int false "页码" minimum(1) default(1)
// @Param pageSize query int false "每页记录数" Enums(10,20,50,100) default(20)
// @Param health query string false "按成员集群聚合健康状态筛选" Enums(ok,degraded,unavailable)
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.PlatformOverviewView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /platform/overview [get]
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

// @ID getPlatformInstanceTrend
// @Summary 获取平台实例趋势
// @Description 查询指定平台组件实例在时间范围内的资源趋势。
// @Tags Platform
// @Produce json
// @Param component path string true "平台组件名称" Enums(webhook,operator,gateway,console-api,console-web,postgres)
// @Param instance path string true "平台组件实例名称"
// @Param cluster query string false "成员面组件所属的集群名称；Gateway 平面组件省略"
// @Param from query string false "查询起始时间，RFC3339 格式" format(date-time)
// @Param to query string false "查询结束时间，RFC3339 格式" format(date-time)
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.PlatformTrendView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /platform/instances/{component}/{instance}/trend [get]
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

// @ID listPlatformLeadershipEvents
// @Summary 查询平台领导权事件
// @Description 分页查询平台组件 Leader 变更记录。
// @Tags Platform
// @Produce json
// @Param page query int false "页码" minimum(1) default(1)
// @Param pageSize query int false "每页记录数" Enums(10,20,50,100) default(20)
// @Param component query string false "按发生领导权变更的组件筛选" Enums(webhook,operator,gateway,console-api,console-web,postgres)
// @Param cluster query string false "按关联成员集群筛选；Gateway 平面事件省略"
// @Param from query string false "查询起始时间，RFC3339 格式" format(date-time)
// @Param to query string false "查询结束时间，RFC3339 格式" format(date-time)
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.PlatformLeadershipPageView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /platform/leadership-events [get]
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

// @ID getPlatformSettings
// @Summary 获取平台可观测性设置
// @Description 查询平台组件采样和历史保存配置。
// @Tags Platform
// @Produce json
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.PlatformSettingsView}
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /platform/settings [get]
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

// @ID updatePlatformSettings
// @Summary 更新平台可观测性设置
// @Description 更新平台组件采样和历史保存配置。
// @Tags Platform
// @Accept json
// @Produce json
// @Param body body dto.PlatformSettingsInput true "平台可观测性设置"
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.PlatformSettingsView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /platform/settings [put]
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
