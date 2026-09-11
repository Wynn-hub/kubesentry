package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) ListDistributions(ctx context.Context, p dto.ListDistributionsParams) (dto.ListDistributionsRes, error) {
	data, err := h.svcs.Distribution.List(ctx, p.Page.Or(1), int(p.PageSize.Or(dto.ListDistributionsPageSize20)), string(p.SortBy.Or(dto.ListDistributionsSortByName)), string(p.SortOrder.Or(dto.ListDistributionsSortOrderAsc)))
	if err != nil {
		return objectError(500), nil
	}
	v, err := dtoData[dto.DistributionListResponseData](data)
	if err != nil {
		return nil, err
	}
	return &dto.DistributionListResponse{Code: 0, Message: "success", Data: v}, nil
}

func (h *Handler) CreateDistribution(ctx context.Context, req *dto.DistributionRequest) (dto.CreateDistributionRes, error) {
	var input dto.DistributionInput
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.CreateDistributionBadRequest{Code: 40000, Message: err.Error()}, nil
	}
	if err := h.svcs.Distribution.Create(ctx, input); err != nil {
		return distributionCreateError(err), nil
	}
	return &dto.DistributionNameResponse{Code: 0, Message: "success", Data: dto.DistributionNameData{Name: input.Name}}, nil
}

func (h *Handler) GetDistribution(ctx context.Context, p dto.GetDistributionParams) (dto.GetDistributionRes, error) {
	data, err := h.svcs.Distribution.Get(ctx, p.Name)
	if err != nil {
		return distributionGetError(err), nil
	}
	v, err := dtoData[dto.DistributionDetail](data)
	if err != nil {
		return nil, err
	}
	return &dto.DistributionDetailResponse{Code: 0, Message: "success", Data: v}, nil
}

func (h *Handler) UpdateDistribution(ctx context.Context, req *dto.DistributionRequest, p dto.UpdateDistributionParams) (dto.UpdateDistributionRes, error) {
	var input dto.DistributionInput
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.UpdateDistributionBadRequest{Code: 40000, Message: err.Error()}, nil
	}
	if err := h.svcs.Distribution.Update(ctx, p.Name, input); err != nil {
		return distributionUpdateError(err), nil
	}
	return &dto.DistributionNameResponse{Code: 0, Message: "success", Data: dto.DistributionNameData{Name: p.Name}}, nil
}

func (h *Handler) DeleteDistribution(ctx context.Context, p dto.DeleteDistributionParams) (dto.DeleteDistributionRes, error) {
	if err := h.svcs.Distribution.Delete(ctx, p.Name); err != nil {
		return distributionDeleteError(err), nil
	}
	return &dto.DistributionEmptyResponse{Code: 0, Message: "success"}, nil
}

func distributionCreateError(err error) dto.CreateDistributionRes {
	if _, ok := err.(*service.ValidationError); ok {
		return &dto.CreateDistributionBadRequest{Code: 40000, Message: err.Error()}
	}
	if service.IsConflict(err) {
		return &dto.CreateDistributionConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.CreateDistributionInternalServerError{Code: 50000, Message: "request failed"}
}

func distributionGetError(err error) dto.GetDistributionRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.GetDistributionNotFound{Code: 40400, Message: "distribution not found"}
	}
	return &dto.GetDistributionInternalServerError{Code: 50000, Message: "request failed"}
}

func distributionUpdateError(err error) dto.UpdateDistributionRes {
	if _, ok := err.(*service.ValidationError); ok {
		return &dto.UpdateDistributionBadRequest{Code: 40000, Message: err.Error()}
	}
	if errors.Is(err, service.ErrNotFound) {
		return &dto.UpdateDistributionNotFound{Code: 40400, Message: "distribution not found"}
	}
	if service.IsConflict(err) {
		return &dto.UpdateDistributionConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.UpdateDistributionInternalServerError{Code: 50000, Message: "request failed"}
}

func distributionDeleteError(err error) dto.DeleteDistributionRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.DeleteDistributionNotFound{Code: 40400, Message: "distribution not found"}
	}
	if service.IsConflict(err) {
		return &dto.DeleteDistributionForbidden{Code: 40300, Message: err.Error()}
	}
	return &dto.DeleteDistributionInternalServerError{Code: 50000, Message: "request failed"}
}
