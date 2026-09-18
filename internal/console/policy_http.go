package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerPolicyRoutes(mux *http.ServeMux, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/policies/validate", policyMethodNotAllowed("POST"))
	mux.HandleFunc("PUT /api/v1/policies/validate", policyMethodNotAllowed("POST"))
	mux.HandleFunc("DELETE /api/v1/policies/validate", policyMethodNotAllowed("POST"))
	mux.HandleFunc("PUT /api/v1/policies/resource-suggestions", policyMethodNotAllowed("GET"))
	mux.HandleFunc("DELETE /api/v1/policies/resource-suggestions", policyMethodNotAllowed("GET"))
	mux.HandleFunc("GET /api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,POST") {
			return
		}
		params, err := decodeListPoliciesQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListPolicies(requestContext(r), params)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/policies", func(w http.ResponseWriter, r *http.Request) {
		var body dto.PolicyRequest
		if !decodeJSON(w, r, "CreatePolicy", &body) {
			return
		}
		response, err := h.CreatePolicy(requestContext(r), &body)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/policies/resource-suggestions", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		response, err := h.GetResourceSuggestions(requestContext(r))
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/policies/validate", func(w http.ResponseWriter, r *http.Request) {
		var body dto.ValidateRegoRequest
		if !decodeJSON(w, r, "ValidatePolicyRego", &body) {
			return
		}
		response, err := h.ValidatePolicyRego(requestContext(r), &body)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/policies/{name}", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,PUT,DELETE") {
			return
		}
		response, err := h.GetPolicy(requestContext(r), dto.GetPolicyParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/policies/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body dto.PolicyRequest
		if !decodeJSON(w, r, "UpdatePolicy", &body) {
			return
		}
		response, err := h.UpdatePolicy(requestContext(r), &body, dto.UpdatePolicyParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("DELETE /api/v1/policies/{name}", func(w http.ResponseWriter, r *http.Request) {
		force, err := queryBool(r.URL.Query(), "force")
		if err != nil {
			writeErr(w, http.StatusBadRequest, policyQueryError("DeletePolicy", "force", err).Error())
			return
		}
		params := dto.DeletePolicyParams{Name: r.PathValue("name")}
		if force != nil {
			params.Force.SetTo(*force)
		}
		response, err := h.DeletePolicy(requestContext(r), params)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/policies/{name}/versions", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		response, err := h.ListPolicyVersions(requestContext(r), dto.ListPolicyVersionsParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/policies/{name}/rollback", func(w http.ResponseWriter, r *http.Request) {
		var body dto.RollbackRequest
		if !decodeJSON(w, r, "RollbackPolicy", &body) {
			return
		}
		response, err := h.RollbackPolicy(requestContext(r), &body, dto.RollbackPolicyParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
}

func policyMethodNotAllowed(allowed string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", allowed)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func rejectHead(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method != http.MethodHead {
		return false
	}
	w.Header().Set("Allow", allowed)
	w.WriteHeader(http.StatusMethodNotAllowed)
	return true
}

func requestContext(r *http.Request) context.Context {
	return context.WithValue(r.Context(), traceIDKey{}, TraceIDFromRequest(r))
}

type consoleJSONBody interface {
	*dto.PolicyRequest | *dto.RollbackRequest | *dto.ValidateRegoRequest | *dto.PolicyGroupRequest | *dto.PolicyGroupEnabledRequest | *dto.ExceptionRequest | *dto.AdmissionEventSettingsRequest | *dto.ClusterCreateRequest | *dto.ClusterLabelsRequest | *dto.ClusterTokenRequest
	UnmarshalJSON([]byte) error
}

func decodeJSON[T consoleJSONBody](w http.ResponseWriter, r *http.Request, operation string, dst T) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation %s: decode request: parse media type: %v", operation, err))
		return false
	}
	if mediaType != "application/json" {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation %s: decode request: unexpected Content-Type: %s", operation, mediaType))
		return false
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeDecodeError(w, operation, err.Error())
		return false
	}
	if len(raw) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation %s: decode request: body required", operation))
		return false
	}
	if err := dst.UnmarshalJSON(raw); err != nil {
		writeDecodeError(w, operation, err.Error())
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		writeDecodeError(w, operation, err.Error())
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeDecodeError(w, operation, "unexpected trailing data")
		return false
	}
	if validator, ok := any(dst).(interface{ Validate() error }); ok {
		if err := validator.Validate(); err != nil {
			writeDecodeError(w, operation, err.Error())
			return false
		}
	}
	return true
}

func writeDecodeError(w http.ResponseWriter, operation, message string) {
	writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation %s: decode request: decode application/json: %s", operation, message))
}

func decodeListPoliciesQuery(query url.Values) (dto.ListPoliciesParams, error) {
	var params dto.ListPoliciesParams
	if value, ok, err := scalarQueryValue(query, "page"); err != nil {
		return params, policyQueryError("ListPolicies", "page", err)
	} else if ok {
		page, err := strconv.Atoi(value)
		if err != nil {
			return params, policyQueryError("ListPolicies", "page", err)
		}
		if page < 1 {
			return params, policyQueryError("ListPolicies", "page", errors.New("invalid: int: value less than minimum: 1"))
		}
		params.Page.SetTo(page)
	}
	if value, ok, err := scalarQueryValue(query, "pageSize"); err != nil {
		return params, policyQueryError("ListPolicies", "pageSize", err)
	} else if ok {
		pageSize, err := strconv.Atoi(value)
		if err != nil {
			return params, policyQueryError("ListPolicies", "pageSize", err)
		}
		parsed := dto.PageSize(pageSize)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListPolicies", "pageSize", err)
		}
		params.PageSize.SetTo(parsed)
	}
	if value, ok, err := scalarQueryValue(query, "sortBy"); err != nil {
		return params, policyQueryError("ListPolicies", "sortBy", err)
	} else if ok {
		parsed := dto.ListPoliciesSortBy(value)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListPolicies", "sortBy", err)
		}
		params.SortBy.SetTo(parsed)
	}
	if value, ok, err := scalarQueryValue(query, "sortOrder"); err != nil {
		return params, policyQueryError("ListPolicies", "sortOrder", err)
	} else if ok {
		parsed := dto.ListPoliciesSortOrder(value)
		if err := parsed.Validate(); err != nil {
			return params, policyQueryError("ListPolicies", "sortOrder", err)
		}
		params.SortOrder.SetTo(parsed)
	}
	if value, ok, err := csvQueryValue(query, "phase"); err != nil {
		return params, policyQueryError("ListPolicies", "phase", err)
	} else if ok && value != "" {
		for _, item := range strings.Split(value, ",") {
			parsed := dto.ListPoliciesPhaseItem(item)
			if err := parsed.Validate(); err != nil {
				return params, policyQueryError("ListPolicies", "phase", fmt.Errorf("invalid: [%d] (%w)", len(params.Phase), err))
			}
			params.Phase = append(params.Phase, parsed)
		}
	}
	if value, ok, err := csvQueryValue(query, "mode"); err != nil {
		return params, policyQueryError("ListPolicies", "mode", err)
	} else if ok && value != "" {
		for _, item := range strings.Split(value, ",") {
			parsed := dto.ListPoliciesModeItem(item)
			if err := parsed.Validate(); err != nil {
				return params, policyQueryError("ListPolicies", "mode", fmt.Errorf("invalid: [%d] (%w)", len(params.Mode), err))
			}
			params.Mode = append(params.Mode, parsed)
		}
	}
	if value, ok, err := csvQueryValue(query, "source"); err != nil {
		return params, policyQueryError("ListPolicies", "source", err)
	} else if ok && value != "" {
		for _, item := range strings.Split(value, ",") {
			parsed := dto.ListPoliciesSourceItem(item)
			if err := parsed.Validate(); err != nil {
				return params, policyQueryError("ListPolicies", "source", fmt.Errorf("invalid: [%d] (%w)", len(params.Source), err))
			}
			params.Source = append(params.Source, parsed)
		}
	}
	if value, ok, err := scalarQueryValue(query, "keyword"); err != nil {
		return params, policyQueryError("ListPolicies", "keyword", err)
	} else if ok {
		params.Keyword.SetTo(value)
	}
	return params, nil
}

func queryBool(query url.Values, name string) (*bool, error) {
	value, ok, err := scalarQueryValue(query, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func scalarQueryValue(query url.Values, name string) (string, bool, error) {
	values, ok := query[name]
	if !ok || len(values) == 0 {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, fmt.Errorf("query parameter %q multiple values", name)
	}
	return values[0], true, nil
}

func csvQueryValue(query url.Values, name string) (string, bool, error) {
	values, ok := query[name]
	if !ok || len(values) == 0 {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, errors.New("invalid value")
	}
	return values[0], true, nil
}

func policyQueryError(operation, name string, err error) error {
	return fmt.Errorf("operation %s: decode params: query: %q: %w", operation, name, err)
}

func writeResponse(w http.ResponseWriter, response any, err error) {
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "request failed")
		return
	}
	status := responseStatus(response)
	if status == 0 {
		writeErr(w, http.StatusInternalServerError, "request failed")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func responseStatus(response any) int {
	switch response.(type) {
	case *dto.PolicyListResponse, *dto.PolicyNameResponse, *dto.PolicyDetailResponse,
		*dto.ResourceSuggestionsResponse, *dto.Envelope, *dto.PolicyVersionListResponse,
		*dto.RollbackResponse, *dto.PolicyGroupListResponse, *dto.PolicyGroupNameResponse,
		*dto.PolicyGroupDetailResponse, *dto.PolicyGroupEnabledResponse,
		*dto.ExceptionListResponse, *dto.ExceptionNameResponse, *dto.ExceptionDetailResponse,
		*dto.FieldSchemaResponse, *dto.SummaryResponse, *dto.AdmissionEventListResponse,
		*dto.AdmissionEventDetailResponse, *dto.AdmissionEventTrendResponse,
		*dto.AdmissionEventSettingsResponse, *dto.AdmissionLinkIncidentListResponse,
		*dto.AdmissionLinkIncidentDetailResponse, *dto.ClusterListResponse,
		*dto.ClusterCommandResponse, *dto.ClusterDetailResponse, *dto.ClusterNameResponse,
		*dto.EmptyResponse, *dto.InstallParamsResponse, *dto.UninstallTaskResponse,
		*dto.UninstallTaskListResponse, *dto.UninstallTaskIDResponse,
		*dto.InstallRecordListResponse:
		return http.StatusOK
	case *dto.ListPoliciesBadRequest, *dto.CreatePolicyBadRequest, *dto.UpdatePolicyBadRequest,
		*dto.RollbackPolicyBadRequest, *dto.ErrorResponse, *dto.GetResourceSuggestionsBadRequest,
		*dto.ListPolicyGroupsBadRequest, *dto.CreatePolicyGroupBadRequest,
		*dto.UpdatePolicyGroupBadRequest, *dto.SetPolicyGroupEnabledBadRequest,
		*dto.ListExceptionsBadRequest, *dto.CreateExceptionBadRequest,
		*dto.UpdateExceptionBadRequest:
		return http.StatusBadRequest
	case *dto.GetFieldSchemaBadRequest:
		return http.StatusBadRequest
	case *dto.ListAdmissionEventsBadRequest, *dto.GetAdmissionEventBadRequest,
		*dto.GetAdmissionEventTrendBadRequest, *dto.UpdateAdmissionEventSettingsBadRequest,
		*dto.ListAdmissionLinkIncidentsBadRequest, *dto.GetAdmissionLinkIncidentBadRequest:
		return http.StatusBadRequest
	case *dto.CreateClusterBadRequest:
		return http.StatusBadRequest
	case *dto.UpdatePolicyGroupForbidden, *dto.DeletePolicyGroupForbidden:
		return http.StatusForbidden
	case *dto.DeleteClusterForbidden:
		return http.StatusForbidden
	case *dto.GetPolicyNotFound, *dto.UpdatePolicyNotFound, *dto.DeletePolicyNotFound,
		*dto.ListPolicyVersionsNotFound, *dto.RollbackPolicyNotFound,
		*dto.GetPolicyGroupNotFound, *dto.UpdatePolicyGroupNotFound,
		*dto.DeletePolicyGroupNotFound, *dto.SetPolicyGroupEnabledNotFound,
		*dto.GetExceptionNotFound, *dto.UpdateExceptionNotFound, *dto.DeleteExceptionNotFound:
		return http.StatusNotFound
	case *dto.GetAdmissionEventNotFound, *dto.GetAdmissionLinkIncidentNotFound:
		return http.StatusNotFound
	case *dto.GetClusterNotFound, *dto.UpdateClusterNotFound, *dto.DeleteClusterNotFound,
		*dto.ReissueClusterTokenNotFound, *dto.GetLatestInstallParamsNotFound,
		*dto.CreateUninstallTaskNotFound, *dto.ListUninstallTasksNotFound,
		*dto.RetryUninstallTaskNotFound:
		return http.StatusNotFound
	case *dto.CreatePolicyConflict, *dto.UpdatePolicyConflict, *dto.DeletePolicyConflict,
		*dto.RollbackPolicyConflict, *dto.CreatePolicyGroupConflict,
		*dto.UpdatePolicyGroupConflict, *dto.SetPolicyGroupEnabledConflict,
		*dto.CreateExceptionConflict, *dto.UpdateExceptionConflict:
		return http.StatusConflict
	case *dto.CreateClusterConflict, *dto.DeleteClusterConflict,
		*dto.ReissueClusterTokenConflict, *dto.CreateUninstallTaskConflict,
		*dto.RetryUninstallTaskConflict:
		return http.StatusConflict
	case *dto.RollbackPolicyGone:
		return http.StatusGone
	case *dto.GetFieldSchemaBadGateway:
		return http.StatusBadGateway
	case *dto.ListAdmissionLinkIncidentsServiceUnavailable,
		*dto.GetAdmissionLinkIncidentServiceUnavailable,
		*dto.GetAdmissionEventSettingsServiceUnavailable:
		return http.StatusServiceUnavailable
	case *dto.ListPoliciesInternalServerError, *dto.CreatePolicyInternalServerError,
		*dto.GetPolicyInternalServerError, *dto.UpdatePolicyInternalServerError,
		*dto.DeletePolicyInternalServerError, *dto.ListPolicyVersionsInternalServerError,
		*dto.RollbackPolicyInternalServerError, *dto.GetResourceSuggestionsInternalServerError,
		*dto.ListPolicyGroupsInternalServerError, *dto.CreatePolicyGroupInternalServerError,
		*dto.GetPolicyGroupInternalServerError, *dto.UpdatePolicyGroupInternalServerError,
		*dto.DeletePolicyGroupInternalServerError, *dto.SetPolicyGroupEnabledInternalServerError,
		*dto.ListExceptionsInternalServerError, *dto.CreateExceptionInternalServerError,
		*dto.GetExceptionInternalServerError, *dto.UpdateExceptionInternalServerError,
		*dto.DeleteExceptionInternalServerError:
		return http.StatusInternalServerError
	case *dto.ListAdmissionEventsInternalServerError, *dto.GetAdmissionEventInternalServerError,
		*dto.GetAdmissionEventTrendInternalServerError,
		*dto.GetAdmissionEventSettingsInternalServerError,
		*dto.UpdateAdmissionEventSettingsInternalServerError,
		*dto.ListAdmissionLinkIncidentsInternalServerError,
		*dto.GetAdmissionLinkIncidentInternalServerError:
		return http.StatusInternalServerError
	case *dto.CreateClusterInternalServerError, *dto.GetClusterInternalServerError,
		*dto.UpdateClusterInternalServerError, *dto.DeleteClusterInternalServerError,
		*dto.ReissueClusterTokenInternalServerError,
		*dto.GetLatestInstallParamsInternalServerError,
		*dto.CreateUninstallTaskInternalServerError,
		*dto.ListUninstallTasksInternalServerError,
		*dto.RetryUninstallTaskInternalServerError:
		return http.StatusInternalServerError
	default:
		return 0
	}
}
