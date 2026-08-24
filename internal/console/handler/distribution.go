package handler

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func (h *Handler) ListDistributions(ctx context.Context) (dto.ListDistributionsRes, error) {
	v, status, err := h.invokeObject(ctx, "GET", "/api/v1/distributions", nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return objectError(status), nil
	}
	return (*dto.ListDistributionsOK)(&v), nil
}

func (h *Handler) CreateDistribution(ctx context.Context, req dto.CreateDistributionReq) (dto.CreateDistributionRes, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	v, status, err := h.invokeObject(ctx, "POST", "/api/v1/distributions", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return objectError(status), nil
	}
	return (*dto.CreateDistributionOK)(&v), nil
}

func (h *Handler) GetDistribution(ctx context.Context, p dto.GetDistributionParams) (dto.GetDistributionRes, error) {
	v, status, err := h.invokeObject(ctx, "GET", "/api/v1/distributions/"+url.PathEscape(p.Name), nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return objectError(status), nil
	}
	return (*dto.GetDistributionOK)(&v), nil
}

func (h *Handler) UpdateDistribution(ctx context.Context, req dto.UpdateDistributionReq, p dto.UpdateDistributionParams) (dto.UpdateDistributionRes, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	v, status, err := h.invokeObject(ctx, "PUT", "/api/v1/distributions/"+url.PathEscape(p.Name), body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return objectError(status), nil
	}
	return (*dto.UpdateDistributionOK)(&v), nil
}

func (h *Handler) DeleteDistribution(ctx context.Context, p dto.DeleteDistributionParams) (dto.DeleteDistributionRes, error) {
	v, status, err := h.invokeObject(ctx, "DELETE", "/api/v1/distributions/"+url.PathEscape(p.Name), nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return objectError(status), nil
	}
	return (*dto.DeleteDistributionOK)(&v), nil
}
