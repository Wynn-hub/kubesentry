package console

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerExceptionRoutes(mux routeRegistrar, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/exceptions", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,POST") {
			return
		}
		params, err := decodeListExceptionsQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListExceptions(requestContext(r), params)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/exceptions", func(w http.ResponseWriter, r *http.Request) {
		var body dto.ExceptionInput
		if !decodeJSON(w, r, "CreateException", &body) {
			return
		}
		response, err := h.CreateException(requestContext(r), &body)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/exceptions/{name}", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "DELETE,GET,PUT") {
			return
		}
		response, err := h.GetException(requestContext(r), dto.GetExceptionParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/exceptions/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body dto.ExceptionInput
		if !decodeJSON(w, r, "UpdateException", &body) {
			return
		}
		response, err := h.UpdateException(requestContext(r), &body, dto.UpdateExceptionParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("DELETE /api/v1/exceptions/{name}", func(w http.ResponseWriter, r *http.Request) {
		response, err := h.DeleteException(requestContext(r), dto.DeleteExceptionParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
}

func decodeListExceptionsQuery(query url.Values) (dto.ListExceptionsParams, error) {
	params := dto.ListExceptionsParams{
		Page:      dto.NewOptInt(1),
		PageSize:  dto.NewOptPageSize(dto.PageSize20),
		SortBy:    dto.NewOptListExceptionsSortBy(dto.ListExceptionsSortByName),
		SortOrder: dto.NewOptListExceptionsSortOrder(dto.ListExceptionsSortOrderAsc),
	}
	if value, ok, err := scalarQueryValue(query, "page"); err != nil {
		return params, policyQueryError("ListExceptions", "page", err)
	} else if ok {
		page, err := strconv.Atoi(value)
		if err != nil {
			return params, policyQueryError("ListExceptions", "page", err)
		}
		if page < 1 {
			return params, policyQueryError("ListExceptions", "page", fmt.Errorf("int: value %d less than 1", page))
		}
		params.Page.SetTo(page)
	}
	if value, ok, err := scalarQueryValue(query, "pageSize"); err != nil {
		return params, policyQueryError("ListExceptions", "pageSize", err)
	} else if ok {
		pageSize, err := strconv.Atoi(value)
		if err != nil {
			return params, policyQueryError("ListExceptions", "pageSize", err)
		}
		parsed := dto.PageSize(pageSize)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListExceptions", "pageSize", err)
		}
		params.PageSize.SetTo(parsed)
	}
	if value, ok, err := scalarQueryValue(query, "sortBy"); err != nil {
		return params, policyQueryError("ListExceptions", "sortBy", err)
	} else if ok {
		parsed := dto.ListExceptionsSortBy(value)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListExceptions", "sortBy", err)
		}
		params.SortBy.SetTo(parsed)
	}
	if value, ok, err := scalarQueryValue(query, "sortOrder"); err != nil {
		return params, policyQueryError("ListExceptions", "sortOrder", err)
	} else if ok {
		parsed := dto.ListExceptionsSortOrder(value)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListExceptions", "sortOrder", err)
		}
		params.SortOrder.SetTo(parsed)
	}
	return params, nil
}
