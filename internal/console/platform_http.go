package console

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerPlatformRoutes(mux *http.ServeMux, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/platform/overview", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		p, err := decodePlatformOverviewQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.GetPlatformOverview(requestContext(r), p)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/platform/instances/{component}/{instance}/trend", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		component := dto.GetPlatformInstanceTrendComponent(r.PathValue("component"))
		if err := component.Validate(); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation GetPlatformInstanceTrend: decode params: path: \"component\": %v", err))
			return
		}
		p := dto.GetPlatformInstanceTrendParams{Component: component, Instance: r.PathValue("instance")}
		if err := setScalar(r.URL.Query(), "GetPlatformInstanceTrend", "cluster", func(v string) error { p.Cluster.SetTo(v); return nil }); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := setOptionalTime(r.URL.Query(), "GetPlatformInstanceTrend", "from", &p.From); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := setOptionalTime(r.URL.Query(), "GetPlatformInstanceTrend", "to", &p.To); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.GetPlatformInstanceTrend(requestContext(r), p)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/platform/leadership-events", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		p, err := decodeLeadershipEventsQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListPlatformLeadershipEvents(requestContext(r), p)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/platform/settings", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,PUT") {
			return
		}
		response, err := h.GetPlatformSettings(requestContext(r))
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/platform/settings", func(w http.ResponseWriter, r *http.Request) {
		var body dto.PlatformSettingsInput
		if !decodeJSON(w, r, "UpdatePlatformSettings", &body) {
			return
		}
		response, err := h.UpdatePlatformSettings(requestContext(r), &body)
		writeResponse(w, response, err)
	})
}

func decodePlatformOverviewQuery(q url.Values) (dto.GetPlatformOverviewParams, error) {
	p := dto.GetPlatformOverviewParams{Page: dto.NewOptInt(1), PageSize: dto.NewOptPageSize(dto.PageSize20)}
	if err := decodePage(q, "GetPlatformOverview", &p.Page, &p.PageSize); err != nil {
		return p, err
	}
	if err := setScalar(q, "GetPlatformOverview", "health", func(v string) error {
		x := dto.GetPlatformOverviewHealth(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.Health.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	return p, nil
}

func decodeLeadershipEventsQuery(q url.Values) (dto.ListPlatformLeadershipEventsParams, error) {
	p := dto.ListPlatformLeadershipEventsParams{Page: dto.NewOptInt(1), PageSize: dto.NewOptPageSize(dto.PageSize20)}
	if err := decodePage(q, "ListPlatformLeadershipEvents", &p.Page, &p.PageSize); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListPlatformLeadershipEvents", "component", func(v string) error {
		x := dto.ListPlatformLeadershipEventsComponent(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.Component.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListPlatformLeadershipEvents", "cluster", func(v string) error { p.Cluster.SetTo(v); return nil }); err != nil {
		return p, err
	}
	if err := setOptionalTime(q, "ListPlatformLeadershipEvents", "from", &p.From); err != nil {
		return p, err
	}
	if err := setOptionalTime(q, "ListPlatformLeadershipEvents", "to", &p.To); err != nil {
		return p, err
	}
	return p, nil
}
