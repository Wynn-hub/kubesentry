package handler

import (
	"context"
	"errors"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// @ID listAdmissionLinkIncidents
// @Summary 分页查询准入链路异常
// @Description 按集群、组件、原因和状态检索准入事件链路异常。
// @Tags LinkIncident
// @Produce json
// @Param page query int false "页码，从 1 开始。省略时为 1" minimum(1) default(1)
// @Param pageSize query int false "每页记录数。仅接受 10、20、50、100。省略时为 20" Enums(10,20,50,100) default(20)
// @Param cluster query []string false "按集群名称筛选，逗号分隔" collectionFormat(csv)
// @Param component query []string false "按组件筛选，逗号分隔" collectionFormat(csv) Enums(webhook,operator,gateway)
// @Param reason query []string false "按原因筛选，逗号分隔" collectionFormat(csv)
// @Param status query []string false "按状态筛选，逗号分隔" collectionFormat(csv) Enums(active,interrupted,closed)
// @Param from query string false "起始时间（RFC3339）" format(date-time)
// @Param to query string false "结束时间（RFC3339）" format(date-time)
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.IncidentPageView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Failure 503 {object} dto.SwagErrorResponse
// @Router /admission-link-incidents [get]
func (h *Handler) ListAdmissionLinkIncidents(ctx context.Context, p dto.ListAdmissionLinkIncidentsParams) (dto.ListAdmissionLinkIncidentsRes, error) {
	components := make([]string, len(p.Component))
	for i := range p.Component {
		components[i] = string(p.Component[i])
	}
	statuses := make([]string, len(p.Status))
	for i := range p.Status {
		statuses[i] = string(p.Status[i])
	}
	var from, to *time.Time
	if value, ok := p.From.Get(); ok {
		from = &value
	}
	if value, ok := p.To.Get(); ok {
		to = &value
	}
	d, err := h.svcs.Incident.List(ctx, dto.IncidentListOptions{Page: p.Page.Or(1), PageSize: int(p.PageSize.Or(dto.PageSize20)), Clusters: p.Cluster, Components: components, Reasons: p.Reason, Statuses: statuses, From: from, To: to})
	if err != nil {
		if errors.Is(err, service.ErrUnavailable) {
			return &dto.ListAdmissionLinkIncidentsServiceUnavailable{Code: 50300, Message: err.Error()}, nil
		}
		if _, ok := err.(*service.ValidationError); ok {
			return &dto.ListAdmissionLinkIncidentsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.ListAdmissionLinkIncidentsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	data, err := dtoData[dto.AdmissionLinkIncidentListData](d)
	if err != nil {
		return nil, err
	}
	return &dto.AdmissionLinkIncidentListResponse{Code: 0, Message: "success", Data: dto.NewOptAdmissionLinkIncidentListData(data)}, nil
}

// @ID getAdmissionLinkIncident
// @Summary 查询准入链路异常详情
// @Description 查询单条准入事件链路异常及其样本事件。
// @Tags LinkIncident
// @Produce json
// @Param id path dto.AdmissionLinkIncidentPath true "准入链路异常记录 ID"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.IncidentView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Failure 503 {object} dto.SwagErrorResponse
// @Router /admission-link-incidents/{id} [get]
func (h *Handler) GetAdmissionLinkIncident(ctx context.Context, p dto.GetAdmissionLinkIncidentParams) (dto.GetAdmissionLinkIncidentRes, error) {
	d, err := h.svcs.Incident.Get(ctx, string(p.ID))
	if err != nil {
		if errors.Is(err, service.ErrUnavailable) {
			return &dto.GetAdmissionLinkIncidentServiceUnavailable{Code: 50300, Message: err.Error()}, nil
		}
		if errors.Is(err, service.ErrNotFound) {
			return &dto.GetAdmissionLinkIncidentNotFound{Code: 40400, Message: err.Error()}, nil
		}
		if _, ok := err.(*service.ValidationError); ok {
			return &dto.GetAdmissionLinkIncidentBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.GetAdmissionLinkIncidentInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	data, err := dtoData[dto.AdmissionLinkIncidentDetail](d)
	if err != nil {
		return nil, err
	}
	return &dto.AdmissionLinkIncidentDetailResponse{Code: 0, Message: "success", Data: dto.NewOptAdmissionLinkIncidentDetail(data)}, nil
}
