package handler

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

func (h *Handler) ListDistributions(ctx context.Context, p dto.ListDistributionsParams) (dto.ListDistributionsRes, error) {
	q := url.Values{}
	if p.Page.Set {
		q.Set("page", strconv.Itoa(p.Page.Value))
	}
	if p.PageSize.Set {
		q.Set("pageSize", strconv.Itoa(int(p.PageSize.Value)))
	}
	if p.SortBy.Set {
		q.Set("sortBy", string(p.SortBy.Value))
	}
	if p.SortOrder.Set {
		q.Set("sortOrder", string(p.SortOrder.Value))
	}
	path := "/api/v1/distributions"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	v, status, err := h.invokeObject(ctx, "GET", path, nil)
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
		return createDistributionError(status), nil
	}
	return (*dto.CreateDistributionOK)(&v), nil
}

func (h *Handler) GetDistribution(ctx context.Context, p dto.GetDistributionParams) (dto.GetDistributionRes, error) {
	v, status, err := h.invokeObject(ctx, "GET", "/api/v1/distributions/"+url.PathEscape(p.Name), nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return getDistributionError(status), nil
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
		return updateDistributionError(status), nil
	}
	return (*dto.UpdateDistributionOK)(&v), nil
}

func (h *Handler) DeleteDistribution(ctx context.Context, p dto.DeleteDistributionParams) (dto.DeleteDistributionRes, error) {
	v, status, err := h.invokeObject(ctx, "DELETE", "/api/v1/distributions/"+url.PathEscape(p.Name), nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return deleteDistributionError(status), nil
	}
	return (*dto.DeleteDistributionOK)(&v), nil
}
