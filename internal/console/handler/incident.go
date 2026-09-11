package handler

import (
	"context"
	"errors"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

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
