package handler

import (
	"context"

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
		return &dto.ListPoliciesBadRequest{Code: 40000, Message: "invalid page: must be greater than zero"}, nil
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
		return &dto.ListPoliciesInternalServerError{Code: 50000, Message: "request failed"}, nil
	}

	return &dto.PolicyListResponse{
		Code:    0,
		Message: "success",
		Data:    dto.OptPolicyListData{Value: *data, Set: true},
	}, nil
}

// GetPolicy implements getPolicy.
func (h *Handler) GetPolicy(ctx context.Context, params dto.GetPolicyParams) (dto.GetPolicyRes, error) {
	detail, err := h.svcs.Policy.Get(ctx, params.Name)
	if err != nil {
		return mapGetPolicyError(err), nil
	}
	return &dto.PolicyDetailResponse{
		Code:    0,
		Message: "success",
		Data:    dto.OptPolicyDetail{Value: *detail, Set: true},
	}, nil
}

// CreatePolicy implements createPolicy.
func (h *Handler) CreatePolicy(ctx context.Context, req *dto.PolicyRequest) (dto.CreatePolicyRes, error) {
	err := h.svcs.Policy.Create(ctx, req)
	if err != nil {
		return mapCreatePolicyError(err), nil
	}
	return &dto.PolicyNameResponse{
		Code:    0,
		Message: "success",
		Data:    dto.OptPolicyNameData{Value: dto.PolicyNameData{Name: req.Name.Or("")}, Set: true},
	}, nil
}

// UpdatePolicy implements updatePolicy.
func (h *Handler) UpdatePolicy(ctx context.Context, req *dto.PolicyRequest, params dto.UpdatePolicyParams) (dto.UpdatePolicyRes, error) {
	err := h.svcs.Policy.Update(ctx, params.Name, req)
	if err != nil {
		return mapUpdatePolicyError(err), nil
	}
	return &dto.PolicyNameResponse{
		Code:    0,
		Message: "success",
		Data:    dto.OptPolicyNameData{Value: dto.PolicyNameData{Name: params.Name}, Set: true},
	}, nil
}

// DeletePolicy implements deletePolicy.
func (h *Handler) DeletePolicy(ctx context.Context, params dto.DeletePolicyParams) (dto.DeletePolicyRes, error) {
	err := h.svcs.Policy.Delete(ctx, params.Name, params.Force.Or(false))
	if err != nil {
		return mapDeletePolicyError(err), nil
	}
	return &dto.Envelope{Code: 0, Message: "success"}, nil
}

// ListPolicyVersions implements listPolicyVersions.
func (h *Handler) ListPolicyVersions(ctx context.Context, params dto.ListPolicyVersionsParams) (dto.ListPolicyVersionsRes, error) {
	list, err := h.svcs.Policy.ListVersions(ctx, params.Name)
	if err != nil {
		return mapListPolicyVersionsError(err), nil
	}
	return &dto.PolicyVersionListResponse{
		Code:    0,
		Message: "success",
		Data:    dto.OptPolicyVersionList{Value: *list, Set: true},
	}, nil
}

// RollbackPolicy implements rollbackPolicy.
func (h *Handler) RollbackPolicy(ctx context.Context, req *dto.RollbackRequest, params dto.RollbackPolicyParams) (dto.RollbackPolicyRes, error) {
	result, err := h.svcs.Policy.Rollback(ctx, params.Name, string(req.Direction))
	if err != nil {
		return mapRollbackPolicyError(err), nil
	}
	return &dto.RollbackResponse{
		Code:    0,
		Message: "success",
		Data:    dto.OptRollbackResult{Value: *result, Set: true},
	}, nil
}

// ValidatePolicyRego implements validatePolicyRego.
func (h *Handler) ValidatePolicyRego(ctx context.Context, req *dto.ValidateRegoRequest) (dto.ValidatePolicyRegoRes, error) {
	err := h.svcs.Policy.ValidateRego(ctx, req.Rego)
	if err != nil {
		// ValidationError maps to bare ErrorResponse, not *XxxBadRequest
		return &dto.ErrorResponse{Code: 40000, Message: err.Error()}, nil
	}
	return &dto.Envelope{Code: 0, Message: "success"}, nil
}

// GetResourceSuggestions implements getResourceSuggestions.
func (h *Handler) GetResourceSuggestions(ctx context.Context) (dto.GetResourceSuggestionsRes, error) {
	sugg, err := h.svcs.Policy.ResourceSuggestions(ctx)
	if err != nil {
		return &dto.GetResourceSuggestionsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ResourceSuggestionsResponse{
		Code:    0,
		Message: "success",
		Data:    dto.OptResourceSuggestions{Value: *sugg, Set: true},
	}, nil
}
