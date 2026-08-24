package handler

import (
	"context"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func (h *Handler) ListExceptions(ctx context.Context, p dto.ListExceptionsParams) (dto.ListExceptionsRes, error) {
	page := p.Page.Or(1)
	size := int(p.PageSize.Or(dto.PageSize20))
	d, e := h.svcs.Exception.List(ctx, page, size, string(p.SortBy.Or(dto.ListExceptionsSortByName)), string(p.SortOrder.Or(dto.ListExceptionsSortOrderAsc)))
	if e != nil {
		return &dto.ListExceptionsInternalServerError{Code: 50000, Message: e.Error()}, nil
	}
	return &dto.ExceptionListResponse{Code: 0, Message: "success", Data: dto.OptExceptionListData{Value: *d, Set: true}}, nil
}

func (h *Handler) GetException(ctx context.Context, p dto.GetExceptionParams) (dto.GetExceptionRes, error) {
	d, e := h.svcs.Exception.Get(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.GetExceptionNotFound{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.GetExceptionInternalServerError{Code: 50000, Message: e.Error()}, nil
	}
	return &dto.ExceptionDetailResponse{Code: 0, Message: "success", Data: dto.OptExceptionDetail{Value: *d, Set: true}}, nil
}

func (h *Handler) CreateException(ctx context.Context, r *dto.ExceptionRequest) (dto.CreateExceptionRes, error) {
	e := h.svcs.Exception.Create(ctx, r)
	if e != nil {
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.CreateExceptionBadRequest{Code: 40000, Message: e.Error()}, nil
		}
		if errors.Is(e, dbrepo.ErrConflict) {
			return &dto.CreateExceptionConflict{Code: 40900, Message: e.Error()}, nil
		}
		return &dto.CreateExceptionInternalServerError{Code: 50000, Message: e.Error()}, nil
	}
	return &dto.ExceptionNameResponse{Code: 0, Message: "success", Data: dto.OptExceptionNameResponseData{Value: dto.ExceptionNameResponseData{Name: r.Name.Or("")}, Set: true}}, nil
}

func (h *Handler) UpdateException(ctx context.Context, r *dto.ExceptionRequest, p dto.UpdateExceptionParams) (dto.UpdateExceptionRes, error) {
	e := h.svcs.Exception.Update(ctx, p.Name, r)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.UpdateExceptionNotFound{Code: 40400, Message: e.Error()}, nil
		}
		if errors.Is(e, dbrepo.ErrConflict) {
			return &dto.UpdateExceptionConflict{Code: 40900, Message: e.Error()}, nil
		}
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.UpdateExceptionBadRequest{Code: 40000, Message: e.Error()}, nil
		}
		return &dto.UpdateExceptionInternalServerError{Code: 50000, Message: e.Error()}, nil
	}
	return &dto.ExceptionNameResponse{Code: 0, Message: "success", Data: dto.OptExceptionNameResponseData{Value: dto.ExceptionNameResponseData{Name: p.Name}, Set: true}}, nil
}

func (h *Handler) DeleteException(ctx context.Context, p dto.DeleteExceptionParams) (dto.DeleteExceptionRes, error) {
	e := h.svcs.Exception.Delete(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.DeleteExceptionNotFound{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.DeleteExceptionInternalServerError{Code: 50000, Message: e.Error()}, nil
	}
	return &dto.Envelope{Code: 0, Message: "success"}, nil
}
