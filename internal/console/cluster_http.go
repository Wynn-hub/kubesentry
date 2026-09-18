package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	consolehandler "github.com/Wynn-hub/kubesentry/internal/console/handler"
)

func registerClusterRoutes(mux *http.ServeMux, h *consolehandler.Handler) {
	mux.HandleFunc("GET /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET,POST") {
			return
		}
		p, err := decodeClustersQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListClusters(requestContext(r), p)
		writeGenericErrorResponse(w, response, err, http.StatusInternalServerError)
	})
	mux.HandleFunc("POST /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		var body dto.ClusterCreateRequest
		if !decodeJSON(w, r, "CreateCluster", &body) {
			return
		}
		response, err := h.CreateCluster(requestContext(r), &body)
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/clusters/{name}", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "DELETE,GET,PUT") {
			return
		}
		response, err := h.GetCluster(requestContext(r), dto.GetClusterParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("PUT /api/v1/clusters/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body dto.ClusterLabelsRequest
		if !decodeJSON(w, r, "UpdateCluster", &body) {
			return
		}
		response, err := h.UpdateCluster(requestContext(r), &body, dto.UpdateClusterParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("DELETE /api/v1/clusters/{name}", func(w http.ResponseWriter, r *http.Request) {
		response, err := h.DeleteCluster(requestContext(r), dto.DeleteClusterParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/clusters/{name}/reissue-token", func(w http.ResponseWriter, r *http.Request) {
		var body dto.ClusterTokenRequest
		if !decodeJSON(w, r, "ReissueClusterToken", &body) {
			return
		}
		response, err := h.ReissueClusterToken(requestContext(r), &body, dto.ReissueClusterTokenParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/clusters/{name}/latest-install-params", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		response, err := h.GetLatestInstallParams(requestContext(r), dto.GetLatestInstallParamsParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/clusters/{name}/uninstall", func(w http.ResponseWriter, r *http.Request) {
		response, err := h.CreateUninstallTask(requestContext(r), dto.CreateUninstallTaskParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/clusters/{name}/uninstall-tasks", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		response, err := h.ListUninstallTasks(requestContext(r), dto.ListUninstallTasksParams{Name: r.PathValue("name")})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("POST /api/v1/clusters/{name}/uninstall-tasks/{taskID}/retry", func(w http.ResponseWriter, r *http.Request) {
		id := dto.ResourceID(r.PathValue("taskID"))
		if err := id.Validate(); err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("operation RetryUninstallTask: decode params: path: \"taskID\": %v", err))
			return
		}
		response, err := h.RetryUninstallTask(requestContext(r), dto.RetryUninstallTaskParams{Name: r.PathValue("name"), TaskID: id})
		writeResponse(w, response, err)
	})
	mux.HandleFunc("GET /api/v1/install-records", func(w http.ResponseWriter, r *http.Request) {
		if rejectHead(w, r, "GET") {
			return
		}
		p, err := decodeInstallRecordsQuery(r.URL.Query())
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		response, err := h.ListInstallRecords(requestContext(r), p)
		writeGenericErrorResponse(w, response, err, http.StatusInternalServerError)
	})
}

func writeGenericErrorResponse(w http.ResponseWriter, response any, err error, status int) {
	if failure, ok := response.(*dto.ErrorResponse); ok && err == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(failure)
		return
	}
	writeResponse(w, response, err)
}

func decodeClustersQuery(q url.Values) (dto.ListClustersParams, error) {
	p := dto.ListClustersParams{Page: dto.NewOptInt(1), PageSize: dto.NewOptPageSize(dto.PageSize20), SortBy: dto.NewOptListClustersSortBy(dto.ListClustersSortByName), SortOrder: dto.NewOptListClustersSortOrder(dto.ListClustersSortOrderAsc)}
	if err := decodePage(q, "ListClusters", &p.Page, &p.PageSize); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListClusters", "sortBy", func(v string) error {
		x := dto.ListClustersSortBy(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.SortBy.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListClusters", "sortOrder", func(v string) error {
		x := dto.ListClustersSortOrder(v)
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

func decodeInstallRecordsQuery(q url.Values) (dto.ListInstallRecordsParams, error) {
	p := dto.ListInstallRecordsParams{Page: dto.NewOptInt(1), PageSize: dto.NewOptPageSize(dto.PageSize20), SortBy: dto.NewOptListInstallRecordsSortBy(dto.ListInstallRecordsSortByCreatedAt), SortOrder: dto.NewOptListInstallRecordsSortOrder(dto.ListInstallRecordsSortOrderDesc)}
	if err := decodePage(q, "ListInstallRecords", &p.Page, &p.PageSize); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListInstallRecords", "sortBy", func(v string) error {
		x := dto.ListInstallRecordsSortBy(v)
		if err := x.Validate(); err != nil {
			return err
		}
		p.SortBy.SetTo(x)
		return nil
	}); err != nil {
		return p, err
	}
	if err := setScalar(q, "ListInstallRecords", "sortOrder", func(v string) error {
		x := dto.ListInstallRecordsSortOrder(v)
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
