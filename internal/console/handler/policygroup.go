package handler

import (
	"context"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func (h *Handler) ListPolicyGroups(ctx context.Context, p dto.ListPolicyGroupsParams) (dto.ListPolicyGroupsRes, error) {
	page := p.Page.Or(1)
	size := int(p.PageSize.Or(dto.PageSize20))
	if page < 1 {
		return &dto.ListPolicyGroupsBadRequest{Code: 40000, Message: "invalid page"}, nil
	}
	d, e := h.svcs.PolicyGroup.List(ctx, page, size, string(p.SortBy.Or(dto.ListPolicyGroupsSortByName)), string(p.SortOrder.Or(dto.ListPolicyGroupsSortOrderAsc)))
	if e != nil {
		return &dto.ListPolicyGroupsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupListResponse{Code: 0, Message: "success", Data: dto.OptPolicyGroupListData{Value: *d, Set: true}}, nil
}

func (h *Handler) GetPolicyGroup(ctx context.Context, p dto.GetPolicyGroupParams) (dto.GetPolicyGroupRes, error) {
	d, e := h.svcs.PolicyGroup.Get(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.GetPolicyGroupNotFound{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.GetPolicyGroupInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupDetailResponse{Code: 0, Message: "success", Data: dto.OptPolicyGroupDetail{Value: *d, Set: true}}, nil
}

func (h *Handler) CreatePolicyGroup(ctx context.Context, r *dto.PolicyGroupRequest) (dto.CreatePolicyGroupRes, error) {
	e := h.svcs.PolicyGroup.Create(ctx, r)
	if e != nil {
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.CreatePolicyGroupBadRequest{Code: 40000, Message: e.Error()}, nil
		}
		if service.IsConflict(e) || service.IsUniqueViolation(e) {
			return &dto.CreatePolicyGroupConflict{Code: 40900, Message: e.Error()}, nil
		}
		return &dto.CreatePolicyGroupInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupNameResponse{Code: 0, Message: "success", Data: dto.OptPolicyGroupNameData{Value: dto.PolicyGroupNameData{Name: r.Name.Or("")}, Set: true}}, nil
}

func (h *Handler) UpdatePolicyGroup(ctx context.Context, r *dto.PolicyGroupRequest, p dto.UpdatePolicyGroupParams) (dto.UpdatePolicyGroupRes, error) {
	e := h.svcs.PolicyGroup.Update(ctx, p.Name, r)
	if e != nil {
		if p.Name == "default" {
			return &dto.UpdatePolicyGroupForbidden{Code: 40300, Message: e.Error()}, nil
		}
		if errors.Is(e, service.ErrNotFound) {
			return &dto.UpdatePolicyGroupNotFound{Code: 40400, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.UpdatePolicyGroupConflict{Code: 40900, Message: e.Error()}, nil
		}
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.UpdatePolicyGroupBadRequest{Code: 40000, Message: e.Error()}, nil
		}
		return &dto.UpdatePolicyGroupInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupNameResponse{Code: 0, Message: "success", Data: dto.OptPolicyGroupNameData{Value: dto.PolicyGroupNameData{Name: p.Name}, Set: true}}, nil
}

func (h *Handler) SetPolicyGroupEnabled(ctx context.Context, r *dto.PolicyGroupEnabledRequest, p dto.SetPolicyGroupEnabledParams) (dto.SetPolicyGroupEnabledRes, error) {
	e := h.svcs.PolicyGroup.SetEnabled(ctx, p.Name, r.Enabled)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.SetPolicyGroupEnabledNotFound{Code: 40400, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.SetPolicyGroupEnabledConflict{Code: 40900, Message: e.Error()}, nil
		}
		return &dto.SetPolicyGroupEnabledInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupEnabledResponse{Code: 0, Message: "success", Data: dto.OptPolicyGroupEnabledData{Value: dto.PolicyGroupEnabledData{Enabled: r.Enabled}, Set: true}}, nil
}

func (h *Handler) DeletePolicyGroup(ctx context.Context, p dto.DeletePolicyGroupParams) (dto.DeletePolicyGroupRes, error) {
	e := h.svcs.PolicyGroup.Delete(ctx, p.Name)
	if e != nil {
		if p.Name == "default" {
			return &dto.DeletePolicyGroupForbidden{Code: 40300, Message: e.Error()}, nil
		}
		if errors.Is(e, service.ErrNotFound) {
			return &dto.DeletePolicyGroupNotFound{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.DeletePolicyGroupInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.Envelope{Code: 0, Message: "success"}, nil
}
