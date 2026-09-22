package console

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerDistributionRoutes(mux *http.ServeMux, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/distributions", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,POST") {
			return
		}
		p, err := decodeDistributionsQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListDistributions(requestContext(r), p)
		writeGenericErrorResponse(w, response, err, http.StatusInternalServerError)
	})
	mux.HandleFunc("POST /api/v1/distributions", func(w http.ResponseWriter, r *http.Request) {
		var body dto.DistributionInput
		if !decodeJSON(w, r, "CreateDistribution", &body) {
			return
		}
		response, err := h.CreateDistribution(requestContext(r), &body)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/distributions/{name}", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "DELETE,GET,PUT") {
			return
		}
		response, err := h.GetDistribution(requestContext(r), dto.GetDistributionParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/distributions/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body dto.DistributionInput
		if !decodeJSON(w, r, "UpdateDistribution", &body) {
			return
		}
		response, err := h.UpdateDistribution(requestContext(r), &body, dto.UpdateDistributionParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("DELETE /api/v1/distributions/{name}", func(w http.ResponseWriter, r *http.Request) {
		response, err := h.DeleteDistribution(requestContext(r), dto.DeleteDistributionParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
}

func decodeDistributionsQuery(q url.Values) (dto.ListDistributionsParams, error) {
	p := dto.ListDistributionsParams{Page: dto.NewOptInt(1), PageSize: dto.NewOptListDistributionsPageSize(dto.ListDistributionsPageSize20), SortBy: dto.NewOptListDistributionsSortBy(dto.ListDistributionsSortByName), SortOrder: dto.NewOptListDistributionsSortOrder(dto.ListDistributionsSortOrderAsc)}
	if err := setScalar(q, "ListDistributions", "page", func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		if n < 1 {
			return fmt.Errorf("int: value %d less than 1", n)
		}
		p.Page.SetTo(n)
		return nil
	}); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListDistributions", "pageSize", func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		x := dto.ListDistributionsPageSize(n)
		if err := x.Validate(); err != nil {
			return err
		}
		p.PageSize.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListDistributions", "sortBy", func(v string) error {
		x := dto.ListDistributionsSortBy(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.SortBy.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListDistributions", "sortOrder", func(v string) error {
		x := dto.ListDistributionsSortOrder(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.SortOrder.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	return p, nil
}
