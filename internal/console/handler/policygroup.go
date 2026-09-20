package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// @ID listPolicyGroups
// @Summary 分页查询策略组列表
// @Description 分页查询策略组及其当前解析出的策略数量。
// @Tags PolicyGroup
// @Produce json
// @Param page query int false "页码，从 1 开始。省略时为 1" minimum(1) default(1)
// @Param pageSize query int false "每页记录数。仅接受 10、20、50、100。省略时为 20" Enums(10,20,50,100) default(20)
// @Param sortBy query string false "排序字段" Enums(name,displayName,enabled,phase,resolvedCount) default(name)
// @Param sortOrder query string false "排序方向" Enums(asc,desc) default(asc)
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.PolicyGroupPageView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policygroups [get]
func (h *Handler) ListPolicyGroups(ctx context.Context, p dto.ListPolicyGroupsParams) (dto.ListPolicyGroupsRes, error) {
	page := p.Page.Or(1)
	size := int(p.PageSize.Or(dto.PageSize20))
	if page < 1 {
		return &dto.PolicyGroupBadRequestHTTPError{Code: 40000, Message: "invalid page"}, nil
	}
	d, e := h.svcs.PolicyGroup.List(ctx, page, size, string(p.SortBy.Or(dto.ListPolicyGroupsSortByName)), string(p.SortOrder.Or(dto.ListPolicyGroupsSortOrderAsc)))
	if e != nil {
		return &dto.PolicyGroupInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupListHTTPResponse{Code: 0, Message: "success", Data: d}, nil
}

// @ID getPolicyGroup
// @Summary 查询策略组详情
// @Description 查询策略组配置、集群作用范围和解析状态。
// @Tags PolicyGroup
// @Produce json
// @Param name path dto.PolicyGroupPath true "策略组名称"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagPolicyGroupDetail}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policygroups/{name} [get]
func (h *Handler) GetPolicyGroup(ctx context.Context, p dto.GetPolicyGroupParams) (dto.GetPolicyGroupRes, error) {
	d, e := h.svcs.PolicyGroup.Get(ctx, p.Name)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.PolicyGroupNotFoundHTTPError{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.PolicyGroupInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupDetailHTTPResponse{Code: 0, Message: "success", Data: d}, nil
}

// @ID createPolicyGroup
// @Summary 创建策略组
// @Description 创建策略组并配置其集群作用范围和策略选择规则。
// @Tags PolicyGroup
// @Accept json
// @Produce json
// @Param body body dto.SwagPolicyGroupInput true "策略组"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagPolicyGroupName}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policygroups [post]
func (h *Handler) CreatePolicyGroup(ctx context.Context, r *dto.PolicyGroupRequest) (dto.CreatePolicyGroupRes, error) {
	input, e := policyGroupInput(r)
	if e != nil {
		return nil, e
	}
	e = h.svcs.PolicyGroup.Create(ctx, input)
	if e != nil {
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.PolicyGroupBadRequestHTTPError{Code: 40000, Message: e.Error()}, nil
		}
		if service.IsConflict(e) || service.IsUniqueViolation(e) {
			return &dto.PolicyGroupConflictHTTPError{Code: 40900, Message: e.Error()}, nil
		}
		return &dto.PolicyGroupInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupNameHTTPResponse{Code: 0, Message: "success", Data: dto.PolicyGroupNameView{Name: stringOr(input.Name)}}, nil
}

// @ID updatePolicyGroup
// @Summary 更新策略组
// @Description 更新策略组配置及其集群作用范围。
// @Tags PolicyGroup
// @Accept json
// @Produce json
// @Param name path dto.PolicyGroupPath true "策略组名称"
// @Param body body dto.SwagPolicyGroupInput true "策略组"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagPolicyGroupName}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policygroups/{name} [put]
func (h *Handler) UpdatePolicyGroup(ctx context.Context, r *dto.PolicyGroupRequest, p dto.UpdatePolicyGroupParams) (dto.UpdatePolicyGroupRes, error) {
	input, e := policyGroupInput(r)
	if e != nil {
		return nil, e
	}
	e = h.svcs.PolicyGroup.Update(ctx, p.Name, input)
	if e != nil {
		if p.Name == "default" {
			return &dto.PolicyGroupForbiddenHTTPError{Code: 40300, Message: e.Error()}, nil
		}
		if errors.Is(e, service.ErrNotFound) {
			return &dto.PolicyGroupNotFoundHTTPError{Code: 40400, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.PolicyGroupConflictHTTPError{Code: 40900, Message: e.Error()}, nil
		}
		var v *service.ValidationError
		if errors.As(e, &v) {
			return &dto.PolicyGroupBadRequestHTTPError{Code: 40000, Message: e.Error()}, nil
		}
		return &dto.PolicyGroupInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupNameHTTPResponse{Code: 0, Message: "success", Data: dto.PolicyGroupNameView{Name: p.Name}}, nil
}

func policyGroupInput(request *dto.PolicyGroupRequest) (*dto.PolicyGroupInput, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var input dto.PolicyGroupInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

// @ID setPolicyGroupEnabled
// @Summary 设置策略组启用状态
// @Description 启用或关闭指定策略组，不改变其成员选择规则。
// @Tags PolicyGroup
// @Accept json
// @Produce json
// @Param name path dto.PolicyGroupPath true "策略组名称"
// @Param body body dto.SwagPolicyGroupEnabledInput true "启用状态"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagPolicyGroupEnabled}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policygroups/{name}/enabled [put]
func (h *Handler) SetPolicyGroupEnabled(ctx context.Context, r *dto.PolicyGroupEnabledRequest, p dto.SetPolicyGroupEnabledParams) (dto.SetPolicyGroupEnabledRes, error) {
	e := h.svcs.PolicyGroup.SetEnabled(ctx, p.Name, r.Enabled)
	if e != nil {
		if errors.Is(e, service.ErrNotFound) {
			return &dto.PolicyGroupNotFoundHTTPError{Code: 40400, Message: e.Error()}, nil
		}
		if service.IsConflict(e) {
			return &dto.PolicyGroupConflictHTTPError{Code: 40900, Message: e.Error()}, nil
		}
		return &dto.PolicyGroupInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupEnabledHTTPResponse{Code: 0, Message: "success", Data: dto.PolicyGroupEnabledView{Enabled: r.Enabled}}, nil
}

// @ID deletePolicyGroup
// @Summary 删除策略组
// @Description 删除非内置策略组。
// @Tags PolicyGroup
// @Produce json
// @Param name path dto.PolicyGroupPath true "策略组名称"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policygroups/{name} [delete]
func (h *Handler) DeletePolicyGroup(ctx context.Context, p dto.DeletePolicyGroupParams) (dto.DeletePolicyGroupRes, error) {
	e := h.svcs.PolicyGroup.Delete(ctx, p.Name)
	if e != nil {
		if p.Name == "default" {
			return &dto.PolicyGroupForbiddenHTTPError{Code: 40300, Message: e.Error()}, nil
		}
		if errors.Is(e, service.ErrNotFound) {
			return &dto.PolicyGroupNotFoundHTTPError{Code: 40400, Message: e.Error()}, nil
		}
		return &dto.PolicyGroupInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyGroupEmptyHTTPResponse{Code: 0, Message: "success"}, nil
}
