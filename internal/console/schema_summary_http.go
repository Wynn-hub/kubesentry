package console

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerSchemaAndSummaryRoutes(mux *http.ServeMux, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/schema/fields", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		params, err := decodeGetFieldSchemaQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.GetFieldSchema(requestContext(r), params)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/summary", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		response, err := h.GetSummary(requestContext(r))
		writeResponse(w, response, err)
	})
}

func decodeGetFieldSchemaQuery(query url.Values) (dto.GetFieldSchemaParams, error) {
	params := dto.GetFieldSchemaParams{Refresh: dto.NewOptBool(false)}
	for _, field := range []struct {
		name     string
		required bool
		set      func(string)
	}{
		{name: "group", set: func(value string) { params.Group.SetTo(value) }},
		{name: "version", required: true, set: func(value string) { params.Version = value }},
		{name: "resource", required: true, set: func(value string) { params.Resource = value }},
	} {
		value, ok, err := scalarQueryValue(query, field.name)
		if err != nil {
			return params, policyQueryError("GetFieldSchema", field.name, err)
		}
		if !ok {
			if field.required {
				return params, policyQueryError("GetFieldSchema", field.name, fmt.Errorf("query parameter %q not set", field.name))
			}
			continue
		}
		field.set(value)
	}
	refresh, err := queryBool(query, "refresh")
	if err != nil {
		return params, policyQueryError("GetFieldSchema", "refresh", err)
	}
	if refresh != nil {
		params.Refresh.SetTo(*refresh)
	}
	return params, nil
}
