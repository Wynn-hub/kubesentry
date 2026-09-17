package console

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerPolicyGroupRoutes(mux *http.ServeMux, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/policygroups", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,POST") {
			return
		}
		params, err := decodeListPolicyGroupsQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListPolicyGroups(requestContext(r), params)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/policygroups", func(w http.ResponseWriter, r *http.Request) {
		var body dto.PolicyGroupRequest
		if !decodeJSON(w, r, "CreatePolicyGroup", &body) {
			return
		}
		response, err := h.CreatePolicyGroup(requestContext(r), &body)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/policygroups/{name}", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "DELETE,GET,PUT") {
			return
		}
		response, err := h.GetPolicyGroup(requestContext(r), dto.GetPolicyGroupParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/policygroups/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body dto.PolicyGroupRequest
		if !decodeJSON(w, r, "UpdatePolicyGroup", &body) {
			return
		}
		response, err := h.UpdatePolicyGroup(requestContext(r), &body, dto.UpdatePolicyGroupParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("DELETE /api/v1/policygroups/{name}", func(w http.ResponseWriter, r *http.Request) {
		response, err := h.DeletePolicyGroup(requestContext(r), dto.DeletePolicyGroupParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/policygroups/{name}/enabled", func(w http.ResponseWriter, r *http.Request) {
		var body dto.PolicyGroupEnabledRequest
		if !decodeJSON(w, r, "SetPolicyGroupEnabled", &body) {
			return
		}
		response, err := h.SetPolicyGroupEnabled(requestContext(r), &body, dto.SetPolicyGroupEnabledParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
}

func decodeListPolicyGroupsQuery(query url.Values) (dto.ListPolicyGroupsParams, error) {
	params := dto.ListPolicyGroupsParams{
		Page:      dto.NewOptInt(1),
		PageSize:  dto.NewOptPageSize(dto.PageSize20),
		SortBy:    dto.NewOptListPolicyGroupsSortBy(dto.ListPolicyGroupsSortByName),
		SortOrder: dto.NewOptListPolicyGroupsSortOrder(dto.ListPolicyGroupsSortOrderAsc),
	}
	if value, ok, err := scalarQueryValue(query, "page"); err != nil {
		return params, policyQueryError("ListPolicyGroups", "page", err)
	} else if ok {
		page, err := strconv.Atoi(value)
		if err != nil {
			return params, policyQueryError("ListPolicyGroups", "page", err)
		}
		if page < 1 {
			return params, policyQueryError("ListPolicyGroups", "page", fmt.Errorf("int: value %d less than 1", page))
		}
		params.Page.SetTo(page)
	}
	if value, ok, err := scalarQueryValue(query, "pageSize"); err != nil {
		return params, policyQueryError("ListPolicyGroups", "pageSize", err)
	} else if ok {
		pageSize, err := strconv.Atoi(value)
		if err != nil {
			return params, policyQueryError("ListPolicyGroups", "pageSize", err)
		}
		parsed := dto.PageSize(pageSize)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListPolicyGroups", "pageSize", err)
		}
		params.PageSize.SetTo(parsed)
	}
	if value, ok, err := scalarQueryValue(query, "sortBy"); err != nil {
		return params, policyQueryError("ListPolicyGroups", "sortBy", err)
	} else if ok {
		parsed := dto.ListPolicyGroupsSortBy(value)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListPolicyGroups", "sortBy", err)
		}
		params.SortBy.SetTo(parsed)
	}
	if value, ok, err := scalarQueryValue(query, "sortOrder"); err != nil {
		return params, policyQueryError("ListPolicyGroups", "sortOrder", err)
	} else if ok {
		parsed := dto.ListPolicyGroupsSortOrder(value)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListPolicyGroups", "sortOrder", err)
		}
		params.SortOrder.SetTo(parsed)
	}
	return params, nil
}
