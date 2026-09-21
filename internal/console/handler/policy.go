package handler

import (
	"context"
	"encoding/json"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

// ListPolicies implements listPolicies.
// @ID listPolicies
// @Summary List policies
// @Description List policies with pagination, sorting, and filters.
// @Tags Policy
// @Produce json
// @Param page query int false "Page number" minimum(1) default(1)
// @Param pageSize query int false "Page size" Enums(10,20,50,100) default(20)
// @Param sortBy query string false "Sort field" Enums(name,source,enforcementMode,phase,createdAt,updatedAt) default(name)
// @Param sortOrder query string false "Sort direction" Enums(asc,desc) default(asc)
// @Param phase query []string false "Compile phases" collectionFormat(csv) Enums(Ready,Invalid)
// @Param mode query []string false "Enforcement modes" collectionFormat(csv) Enums(enforce,audit)
// @Param source query []string false "Policy sources" collectionFormat(csv) Enums(builtin,custom)
// @Param keyword query string false "Search keyword"
// @Success 200 {object} dto.SwagPolicyListResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies [get]
func (h *Handler) ListPolicies(ctx context.Context, params dto.ListPoliciesParams) (dto.ListPoliciesRes, error) {
	// Validate pagination params (protocol semantics, not business logic)
	page := params.Page.Or(1)
	if page < 1 {
		return &dto.PolicyBadRequestHTTPError{Code: 40000, Message: "invalid page: must be greater than zero"}, nil
	}
	pageSize := int(params.PageSize.Or(dto.PageSize20))

	sortBy := string(params.SortBy.Or(dto.ListPoliciesSortByName))
	sortOrder := string(params.SortOrder.Or(dto.ListPoliciesSortOrderAsc))

	// Map filter enums to strings
	var phases, modes, sources []string
	for _, p := range params.Phase {
		phases = append(phases, string(p))
	}
	for _, m := range params.Mode {
		modes = append(modes, string(m))
	}
	for _, s := range params.Source {
		sources = append(sources, string(s))
	}

	data, err := h.svcs.Policy.List(ctx, service.ListOptions{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
		Phases:    phases,
		Modes:     modes,
		Sources:   sources,
		Keyword:   params.Keyword.Or(""),
	})
	if err != nil {
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.PolicyListHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    data,
	}, nil
}

// GetPolicy implements getPolicy.
// @ID getPolicy
// @Summary 查询策略详情
// @Description 返回策略完整内容，含引用它的策略组列表与乐观并发令牌。
// @Tags Policy
// @Produce json
// @Param name path dto.PolicyPath true "策略名称"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagPolicyDetail}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies/{name} [get]
func (h *Handler) GetPolicy(ctx context.Context, params dto.GetPolicyParams) (dto.GetPolicyRes, error) {
	detail, err := h.svcs.Policy.Get(ctx, params.Name)
	if err != nil {
		return mapGetPolicyError(err), nil
	}
	return &dto.PolicyDetailHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    detail,
	}, nil
}

// CreatePolicy implements createPolicy.
// @ID createPolicy
// @Summary 创建策略
// @Description 创建策略并同时写入 v1 版本快照，两者在同一事务内提交。Rego 在写库前同步编译校验，编译失败返回 40000 且不落库。
// @Tags Policy
// @Accept json
// @Produce json
// @Param body body dto.SwagPolicyInput true "策略"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.PolicyNameView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies [post]
func (h *Handler) CreatePolicy(ctx context.Context, req *dto.PolicyRequest) (dto.CreatePolicyRes, error) {
	input, err := policyInput(req)
	if err != nil {
		return nil, err
	}
	err = h.svcs.Policy.Create(ctx, input)
	if err != nil {
		return mapCreatePolicyError(err), nil
	}
	return &dto.PolicyNameHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    dto.PolicyNameView{Name: stringOr(input.Name)},
	}, nil
}

// UpdatePolicy implements updatePolicy.
// @ID updatePolicy
// @Summary 更新策略
// @Description 更新策略并追加新版本快照，两者在同一事务内提交。labels 与现有值合并；annotations 中值为空字符串的键表示删除。提供 resourceVersion 时做比较并交换，不匹配返回 40900。
// @Tags Policy
// @Accept json
// @Produce json
// @Param name path dto.PolicyPath true "策略名称"
// @Param body body dto.SwagPolicyInput true "策略"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.PolicyNameView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies/{name} [put]
func (h *Handler) UpdatePolicy(ctx context.Context, req *dto.PolicyRequest, params dto.UpdatePolicyParams) (dto.UpdatePolicyRes, error) {
	input, err := policyInput(req)
	if err != nil {
		return nil, err
	}
	err = h.svcs.Policy.Update(ctx, params.Name, input)
	if err != nil {
		return mapUpdatePolicyError(err), nil
	}
	return &dto.PolicyNameHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    dto.PolicyNameView{Name: params.Name},
	}, nil
}

// DeletePolicy implements deletePolicy.
// @ID deletePolicy
// @Summary 删除策略
// @Description 默认拒绝删除被策略组引用的策略，返回 40900 并在 data.referencedBy 列出引用方。force=true 跳过该检查强制删除。
// @Tags Policy
// @Produce json
// @Param name path dto.PolicyPath true "策略名称"
// @Param force query bool false "为 true 时跳过引用检查强制删除" default(false)
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies/{name} [delete]
func (h *Handler) DeletePolicy(ctx context.Context, params dto.DeletePolicyParams) (dto.DeletePolicyRes, error) {
	err := h.svcs.Policy.Delete(ctx, params.Name, params.Force.Or(false))
	if err != nil {
		return mapDeletePolicyError(err), nil
	}
	return &dto.PolicyEmptyHTTPResponse{Code: 0, Message: "success"}, nil
}

// ListPolicyVersions implements listPolicyVersions.
// @ID listPolicyVersions
// @Summary 查询策略版本历史
// @Description 返回版本快照列表，按版本号倒序，并标记当前生效版本。
// @Tags Policy
// @Produce json
// @Param name path dto.PolicyPath true "策略名称"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagPolicyVersionList}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies/{name}/versions [get]
func (h *Handler) ListPolicyVersions(ctx context.Context, params dto.ListPolicyVersionsParams) (dto.ListPolicyVersionsRes, error) {
	list, err := h.svcs.Policy.ListVersions(ctx, params.Name)
	if err != nil {
		return mapListPolicyVersionsError(err), nil
	}
	return &dto.PolicyVersionListHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    list,
	}, nil
}

// RollbackPolicy implements rollbackPolicy.
// @ID rollbackPolicy
// @Summary 回滚策略到上一版本
// @Description 以追加新版本的方式回滚（roll-forward），类似 git revert，不修改历史。direction 为 next 恒返回 40000。已在最早版本时返回 40000。目标版本快照已被保留策略清理时返回 41000。
// @Tags Policy
// @Accept json
// @Produce json
// @Param name path dto.PolicyPath true "策略名称"
// @Param body body dto.SwagRollbackInput true "回滚方向"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagRollbackResult}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 410 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies/{name}/rollback [post]
func (h *Handler) RollbackPolicy(ctx context.Context, req *dto.PolicyRollbackInput, params dto.RollbackPolicyParams) (dto.RollbackPolicyRes, error) {
	result, err := h.svcs.Policy.Rollback(ctx, params.Name, req.Direction)
	if err != nil {
		return mapRollbackPolicyError(err), nil
	}
	return &dto.PolicyRollbackHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    result,
	}, nil
}

// ValidatePolicyRego implements validatePolicyRego.
// @ID validatePolicyRego
// @Summary 校验 Rego 源码
// @Description 只做 Rego 编译校验，不落库、不校验其他字段。编译通过返回 code 0 且无 data，失败返回 40000 并在 message 中给出编译器原文。
// @Tags Policy
// @Accept json
// @Produce json
// @Param body body dto.SwagValidateRegoInput true "Rego 源码"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Router /policies/validate [post]
func (h *Handler) ValidatePolicyRego(ctx context.Context, req *dto.PolicyValidateRegoInput) (dto.ValidatePolicyRegoRes, error) {
	err := h.svcs.Policy.ValidateRego(ctx, req.Rego)
	if err != nil {
		// ValidationError maps to bare ErrorResponse, not *XxxBadRequest
		return &dto.PolicyBadRequestHTTPError{Code: 40000, Message: err.Error()}, nil
	}
	return &dto.PolicyEmptyHTTPResponse{Code: 0, Message: "success"}, nil
}

// GetResourceSuggestions implements getResourceSuggestions.
// @ID getResourceSuggestions
// @Summary 查询资源建议
// @Description 聚合所有策略 match 中用过的 apiGroups / apiVersions / resources 并去重排序，供前端可视化编辑器的下拉建议使用。不读取 rego 字段以避免传输大字段。
// @Tags Policy
// @Produce json
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SwagResourceSuggestions}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /policies/resource-suggestions [get]
func (h *Handler) GetResourceSuggestions(ctx context.Context) (dto.GetResourceSuggestionsRes, error) {
	sugg, err := h.svcs.Policy.ResourceSuggestions(ctx)
	if err != nil {
		return &dto.PolicyInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ResourceSuggestionsHTTPResponse{
		Code:    0,
		Message: "success",
		Data:    sugg,
	}, nil
}

func policyInput(request *dto.PolicyRequest) (*dto.PolicyInput, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	var input dto.PolicyInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return &input, nil
}
