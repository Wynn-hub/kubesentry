package handler

import (
	"context"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// @ID listExceptions
// @Summary 分页查询策略例外
// @Description 分页查询策略例外及其生效状态。
// @Tags Exception
// @Produce json
// @Security SessionCookie
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Param page query int false "页码，从 1 开始。省略时为 1" minimum(1) default(1)
// @Param pageSize query int false "每页记录数。仅接受 10、20、50、100。省略时为 20" Enums(10,20,50,100) default(20)
// @Param sortBy query string false "排序字段" Enums(name,phase,targetSummary,duration,expiresAt) default(name)
// @Param sortOrder query string false "排序方向" Enums(asc,desc) default(asc)
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.ExceptionPageView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /exceptions [get]
func (h *Handler) ListExceptions(ctx context.Context, p dto.ListExceptionsParams) (dto.ListExceptionsRes, error) {
	page := p.Page.Or(1)
	size := int(p.PageSize.Or(dto.PageSize20))
	d, e := h.svcs.Exception.List(ctx, page, size, string(p.SortBy.Or(dto.ListExceptionsSortByName)), string(p.SortOrder.Or(dto.ListExceptionsSortOrderAsc)))
	if e != nil {
		return &dto.ExceptionInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ExceptionListHTTPResponse{Code: 0, Message: "success", Data: d}, nil
}

// @ID getException
// @Summary 查询策略例外详情
// @Description 查询策略例外的匹配条件、作用范围和过期时间。
// @Tags Exception
// @Produce json
// @Security SessionCookie
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Param name path dto.ExceptionPath true "例外名称"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagExceptionDetail}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /exceptions/{name} [get]
func (h *Handler) GetException(ctx context.Context, p dto.GetExceptionParams) (dto.GetExceptionRes, error) {
	d, e := h.svcs.Exception.Get(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.ExceptionNotFoundHTTPError{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.ExceptionInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ExceptionDetailHTTPResponse{Code: 0, Message: "success", Data: d}, nil
}

// @ID createException
// @Summary 创建策略例外
// @Description 创建针对资源、命名空间或集群范围的策略例外。
// @Tags Exception
// @Accept json
// @Produce json
// @Security SessionCookie
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Param body body dto.SwagExceptionInput true "策略例外"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.ExceptionNameView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /exceptions [post]
func (h *Handler) CreateException(ctx context.Context, input *dto.ExceptionInput) (dto.CreateExceptionRes, error) {
	e := h.svcs.Exception.Create(ctx, input)
	if e != nil {
		if errors.Is(e, service.ErrForbidden) {
			return &dto.ExceptionForbiddenHTTPError{Code: 40300, Message: e.Error()}, nil
		}
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.ExceptionBadRequestHTTPError{Code: 40000, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.ExceptionConflictHTTPError{Code: 40900, Message: e.Error()}, nil
		}
		return &dto.ExceptionInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ExceptionNameHTTPResponse{Code: 0, Message: "success", Data: dto.ExceptionNameView{Name: stringOr(input.Name)}}, nil
}

// @ID updateException
// @Summary 更新策略例外
// @Description 更新策略例外的匹配条件和作用范围。
// @Tags Exception
// @Accept json
// @Produce json
// @Security SessionCookie
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Param name path dto.ExceptionPath true "例外名称"
// @Param body body dto.SwagExceptionInput true "策略例外"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.ExceptionNameView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /exceptions/{name} [put]
func (h *Handler) UpdateException(ctx context.Context, input *dto.ExceptionInput, p dto.UpdateExceptionParams) (dto.UpdateExceptionRes, error) {
	e := h.svcs.Exception.Update(ctx, p.Name, input)
	if e != nil {
		if errors.Is(e, service.ErrForbidden) {
			return &dto.ExceptionForbiddenHTTPError{Code: 40300, Message: e.Error()}, nil
		}
		if errors.Is(e, service.ErrNotFound) {
			return &dto.ExceptionNotFoundHTTPError{Code: 40400, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.ExceptionConflictHTTPError{Code: 40900, Message: e.Error()}, nil
		}
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.ExceptionBadRequestHTTPError{Code: 40000, Message: e.Error()}, nil
		}
		return &dto.ExceptionInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ExceptionNameHTTPResponse{Code: 0, Message: "success", Data: dto.ExceptionNameView{Name: p.Name}}, nil
}

func stringOr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// @ID deleteException
// @Summary 删除策略例外
// @Description 删除指定策略例外。
// @Tags Exception
// @Produce json
// @Security SessionCookie
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Param name path dto.ExceptionPath true "例外名称"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /exceptions/{name} [delete]
func (h *Handler) DeleteException(ctx context.Context, p dto.DeleteExceptionParams) (dto.DeleteExceptionRes, error) {
	e := h.svcs.Exception.Delete(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrDefaultExceptionDelete) || errors.Is(e, service.ErrForbidden) {
			return &dto.ExceptionForbiddenHTTPError{Code: 40300, Message: e.Error()}, nil
		}
		if errors.Is(e, service.ErrNotFound) {
			return &dto.ExceptionNotFoundHTTPError{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.ExceptionInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ExceptionEmptyHTTPResponse{Code: 0, Message: "success"}, nil
}
