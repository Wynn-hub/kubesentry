package handler

import (
	"context"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) ListAdmissionLinkIncidents(ctx context.Context, p dto.ListAdmissionLinkIncidentsParams) (dto.ListAdmissionLinkIncidentsRes, error) {
	d, err := h.svcs.Incident.List(ctx, p)
	if err != nil {
		if errors.Is(err, service.ErrUnavailable) {
			return &dto.ListAdmissionLinkIncidentsServiceUnavailable{Code: 50300, Message: err.Error()}, nil
		}
		if _, ok := err.(*service.ValidationError); ok {
			return &dto.ListAdmissionLinkIncidentsBadRequest{Code: 40000, Message: err.Error()}, nil
		}
		return &dto.ListAdmissionLinkIncidentsInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	return &dto.AdmissionLinkIncidentListResponse{Code: 0, Message: "success", Data: dto.OptAdmissionLinkIncidentListData{Value: *d, Set: true}}, nil
}

func (h *Handler) GetAdmissionLinkIncident(ctx context.Context, p dto.GetAdmissionLinkIncidentParams) (dto.GetAdmissionLinkIncidentRes, error) {
	d, err := h.svcs.Incident.Get(ctx, p.ID)
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
		return &dto.GetAdmissionLinkIncidentInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	return &dto.AdmissionLinkIncidentDetailResponse{Code: 0, Message: "success", Data: dto.OptAdmissionLinkIncidentDetail{Value: *d, Set: true}}, nil
}
