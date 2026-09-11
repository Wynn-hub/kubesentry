package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) ListExceptions(ctx context.Context, p dto.ListExceptionsParams) (dto.ListExceptionsRes, error) {
	page := p.Page.Or(1)
	size := int(p.PageSize.Or(dto.PageSize20))
	d, e := h.svcs.Exception.List(ctx, page, size, string(p.SortBy.Or(dto.ListExceptionsSortByName)), string(p.SortOrder.Or(dto.ListExceptionsSortOrderAsc)))
	if e != nil {
		return &dto.ListExceptionsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.ExceptionListData](d)
	if err != nil {
		return nil, err
	}
	return &dto.ExceptionListResponse{Code: 0, Message: "success", Data: dto.NewOptExceptionListData(v)}, nil
}

func (h *Handler) GetException(ctx context.Context, p dto.GetExceptionParams) (dto.GetExceptionRes, error) {
	d, e := h.svcs.Exception.Get(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.GetExceptionNotFound{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.GetExceptionInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	v, err := dtoData[dto.ExceptionDetail](d)
	if err != nil {
		return nil, err
	}
	return &dto.ExceptionDetailResponse{Code: 0, Message: "success", Data: dto.NewOptExceptionDetail(v)}, nil
}

func (h *Handler) CreateException(ctx context.Context, r *dto.ExceptionRequest) (dto.CreateExceptionRes, error) {
	input, e := exceptionInput(r)
	if e != nil {
		return nil, e
	}
	e = h.svcs.Exception.Create(ctx, input)
	if e != nil {
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.CreateExceptionBadRequest{Code: 40000, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.CreateExceptionConflict{Code: 40900, Message: e.Error()}, nil
		}
		return &dto.CreateExceptionInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ExceptionNameResponse{Code: 0, Message: "success", Data: dto.OptExceptionNameResponseData{Value: dto.ExceptionNameResponseData{Name: stringOr(input.Name)}, Set: true}}, nil
}

func (h *Handler) UpdateException(ctx context.Context, r *dto.ExceptionRequest, p dto.UpdateExceptionParams) (dto.UpdateExceptionRes, error) {
	input, e := exceptionInput(r)
	if e != nil {
		return nil, e
	}
	e = h.svcs.Exception.Update(ctx, p.Name, input)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.UpdateExceptionNotFound{Code: 40400, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.UpdateExceptionConflict{Code: 40900, Message: e.Error()}, nil
		}
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.UpdateExceptionBadRequest{Code: 40000, Message: e.Error()}, nil
		}
		return &dto.UpdateExceptionInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ExceptionNameResponse{Code: 0, Message: "success", Data: dto.OptExceptionNameResponseData{Value: dto.ExceptionNameResponseData{Name: p.Name}, Set: true}}, nil
}

func exceptionInput(request *dto.ExceptionRequest) (*dto.ExceptionInput, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var input dto.ExceptionInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

func stringOr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (h *Handler) DeleteException(ctx context.Context, p dto.DeleteExceptionParams) (dto.DeleteExceptionRes, error) {
	e := h.svcs.Exception.Delete(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.DeleteExceptionNotFound{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.DeleteExceptionInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.Envelope{Code: 0, Message: "success"}, nil
}
