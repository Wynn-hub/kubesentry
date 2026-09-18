package console

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerAdmissionRoutes(mux *http.ServeMux, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/admission-events", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		p, err := decodeAdmissionEventsQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListAdmissionEvents(requestContext(r), p)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/admission-events/trend", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		from, err := requiredTimeQuery(r.URL.Query(), "GetAdmissionEventTrend", "from")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		to, err := requiredTimeQuery(r.URL.Query(), "GetAdmissionEventTrend", "to")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.GetAdmissionEventTrend(requestContext(r), dto.GetAdmissionEventTrendParams{From: from, To: to})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/admission-events/{id}", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		id := dto.ResourceID(r.PathValue("id"))
		if err := id.Validate(); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation GetAdmissionEvent: decode params: path: \"id\": %v", err))
			return
		}
		response, err := h.GetAdmissionEvent(requestContext(r), dto.GetAdmissionEventParams{ID: id})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/admission-event-settings", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,PUT") {
			return
		}
		response, err := h.GetAdmissionEventSettings(requestContext(r))
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/admission-event-settings", func(w http.ResponseWriter, r *http.Request) {
		var body dto.AdmissionEventSettingsRequest
		if !decodeJSON(w, r, "UpdateAdmissionEventSettings", &body) {
			return
		}
		response, err := h.UpdateAdmissionEventSettings(requestContext(r), &body)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/admission-link-incidents", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		p, err := decodeIncidentsQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListAdmissionLinkIncidents(requestContext(r), p)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/admission-link-incidents/{id}", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		id := dto.ResourceID(r.PathValue("id"))
		if err := id.Validate(); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation GetAdmissionLinkIncident: decode params: path: \"id\": %v", err))
			return
		}
		response, err := h.GetAdmissionLinkIncident(requestContext(r), dto.GetAdmissionLinkIncidentParams{ID: id})
		writeResponse(w, response, err)
	})
}

func decodeAdmissionEventsQuery(q url.Values) (dto.ListAdmissionEventsParams, error) {
	p := dto.ListAdmissionEventsParams{Page: dto.NewOptInt(1), PageSize: dto.NewOptPageSize(dto.PageSize20), SortBy: dto.NewOptListAdmissionEventsSortBy(dto.ListAdmissionEventsSortByOccurredAt), SortOrder: dto.NewOptListAdmissionEventsSortOrder(dto.ListAdmissionEventsSortOrderDesc)}
	if err := decodePage(q, "ListAdmissionEvents", &p.Page, &p.PageSize); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListAdmissionEvents", "sortBy", func(v string) error {
		x := dto.ListAdmissionEventsSortBy(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.SortBy.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListAdmissionEvents", "sortOrder", func(v string) error {
		x := dto.ListAdmissionEventsSortOrder(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.SortOrder.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	for _, x := range []struct {
		name string
		dst  *[]string
	}{{"cluster", &p.Cluster}, {"namespace", &p.Namespace}, {"gvk", &p.Gvk}} {
		v, ok, err := csvQueryValue(q, x.name)
		if err != nil {
			return p, policyQueryError("ListAdmissionEvents", x.name, err)
		}
		if ok && v != "" {
			*x.dst = strings.Split(v, ",")
		}
	}
	if v, ok, err := csvQueryValue(q, "verdict"); err != nil {
		return p, policyQueryError("ListAdmissionEvents", "verdict", err)
	} else if ok && v != "" {
		for i, s := range strings.Split(v, ",") {
			x := dto.ListAdmissionEventsVerdictItem(s)
			if err := x.Validate(); err != nil {
				return p, policyQueryError("ListAdmissionEvents", "verdict", fmt.Errorf("invalid: [%d] (%w)", i, err))
			}
			p.Verdict = append(p.Verdict, x)
		}
	}
	for _, x := range []struct {
		name string
		set  func(string)
	}{{"policy", func(v string) { p.Policy.SetTo(v) }}, {"resourceName", func(v string) { p.ResourceName.SetTo(v) }}, {"username", func(v string) { p.Username.SetTo(v) }}} {
		if err := setScalar(q, "ListAdmissionEvents", x.name, func(v string) error { x.set(v); return nil }); err != nil {
			return p, err
		}
	}
	if err := setOptionalTime(q, "ListAdmissionEvents", "from", &p.From); err != nil {
		return p, err
	}
	if err := setOptionalTime(q, "ListAdmissionEvents", "to", &p.To); err != nil {
		return p, err
	}
	return p, nil
}

func decodeIncidentsQuery(q url.Values) (dto.ListAdmissionLinkIncidentsParams, error) {
	p := dto.ListAdmissionLinkIncidentsParams{Page: dto.NewOptInt(1), PageSize: dto.NewOptPageSize(dto.PageSize20)}
	if err := decodePage(q, "ListAdmissionLinkIncidents", &p.Page, &p.PageSize); err != nil {
		return p, err
	}
	for _, x := range []struct {
		name string
		dst  *[]string
	}{{"cluster", &p.Cluster}, {"reason", &p.Reason}} {
		v, ok, err := csvQueryValue(q, x.name)
		if err != nil {
			return p, policyQueryError("ListAdmissionLinkIncidents", x.name, err)
		}
		if ok && v != "" {
			*x.dst = strings.Split(v, ",")
		}
	}
	if v, ok, err := csvQueryValue(q, "component"); err != nil {
		return p, policyQueryError("ListAdmissionLinkIncidents", "component", err)
	} else if ok && v != "" {
		for i, s := range strings.Split(v, ",") {
			x := dto.ListAdmissionLinkIncidentsComponentItem(s)
			if err := x.Validate(); err != nil {
				return p, policyQueryError("ListAdmissionLinkIncidents", "component", fmt.Errorf("invalid: [%d] (%w)", i, err))
			}
			p.Component = append(p.Component, x)
		}
	}
	if v, ok, err := csvQueryValue(q, "status"); err != nil {
		return p, policyQueryError("ListAdmissionLinkIncidents", "status", err)
	} else if ok && v != "" {
		for i, s := range strings.Split(v, ",") {
			x := dto.ListAdmissionLinkIncidentsStatusItem(s)
			if err := x.Validate(); err != nil {
				return p, policyQueryError("ListAdmissionLinkIncidents", "status", fmt.Errorf("invalid: [%d] (%w)", i, err))
			}
			p.Status = append(p.Status, x)
		}
	}
	if err := setOptionalTime(q, "ListAdmissionLinkIncidents", "from", &p.From); err != nil {
		return p, err
	}
	if err := setOptionalTime(q, "ListAdmissionLinkIncidents", "to", &p.To); err != nil {
		return p, err
	}
	return p, nil
}

func decodePage(q url.Values, op string, page *dto.OptInt, pageSize *dto.OptPageSize) error {
	if err := setScalar(q, op, "page", func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		if n < 1 {
			return fmt.Errorf("int: value %d less than 1", n)
		}
		page.SetTo(n)
		return nil
	}); err != nil {
		return err
	}
	return setScalar(q, op, "pageSize", func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		x := dto.PageSize(n)
		if err := x.Validate(); err != nil {
			return err
		}
		pageSize.SetTo(x)
		return nil
	})
}

func setScalar(q url.Values, op, name string, set func(string) error) error {
	v, ok, err := scalarQueryValue(q, name)
	if err != nil {
		return policyQueryError(op, name, err)
	}
	if !ok {
		return nil
	}
	if err := set(v); err != nil {
		return policyQueryError(op, name, err)
	}
	return nil
}

func setOptionalTime(q url.Values, op, name string, dst *dto.OptDateTime) error {
	return setScalar(q, op, name, func(v string) error {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		dst.SetTo(parsed)
		return nil
	})
}

func requiredTimeQuery(q url.Values, op, name string) (time.Time, error) {
	v, ok, err := scalarQueryValue(q, name)
	if err != nil {
		return time.Time{}, policyQueryError(op, name, err)
	}
	if !ok {
		return time.Time{}, policyQueryError(op, name, fmt.Errorf("query parameter %q not set", name))
	}
	parsed, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, policyQueryError(op, name, err)
	}
	return parsed, nil
}
