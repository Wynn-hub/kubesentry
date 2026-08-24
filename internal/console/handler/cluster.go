package handler

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/go-faster/jx"
)

func (h *Handler) invokeObject(ctx context.Context, method, path string, body []byte) (map[string]jx.Raw, int, error) {
	return h.platformInvoke(ctx, method, path, body)
}

func objectError(status int) *dto.ErrorResponse {
	if status < 400 {
		status = 500
	}
	return &dto.ErrorResponse{Code: status * 100, Message: "request failed"}
}

func queryPage(q url.Values, page dto.OptInt, size dto.OptPageSize) {
	if page.Set {
		q.Set("page", strconv.Itoa(page.Value))
	}
	if size.Set {
		q.Set("pageSize", strconv.Itoa(int(size.Value)))
	}
}

func (h *Handler) ListClusters(ctx context.Context, p dto.ListClustersParams) (dto.ListClustersRes, error) {
	q := url.Values{}
	queryPage(q, p.Page, p.PageSize)
	path := "/api/v1/clusters"
	if s := q.Encode(); s != "" {
		path += "?" + s
	}
	v, status, err := h.invokeObject(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return objectError(status), nil
	}
	return (*dto.ListClustersOK)(&v), nil
}

func (h *Handler) CreateCluster(ctx context.Context, req dto.CreateClusterReq) (dto.CreateClusterRes, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	v, status, err := h.invokeObject(ctx, "POST", "/api/v1/clusters", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return createClusterError(status), nil
	}
	return (*dto.CreateClusterOK)(&v), nil
}

func (h *Handler) GetCluster(ctx context.Context, p dto.GetClusterParams) (dto.GetClusterRes, error) {
	v, status, err := h.invokeObject(ctx, "GET", "/api/v1/clusters/"+url.PathEscape(p.Name), nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return getClusterError(status), nil
	}
	return (*dto.GetClusterOK)(&v), nil
}

func (h *Handler) UpdateCluster(ctx context.Context, req dto.UpdateClusterReq, p dto.UpdateClusterParams) (dto.UpdateClusterRes, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	v, status, err := h.invokeObject(ctx, "PUT", "/api/v1/clusters/"+url.PathEscape(p.Name), body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return updateClusterError(status), nil
	}
	return (*dto.UpdateClusterOK)(&v), nil
}

func (h *Handler) DeleteCluster(ctx context.Context, p dto.DeleteClusterParams) (dto.DeleteClusterRes, error) {
	v, status, err := h.invokeObject(ctx, "DELETE", "/api/v1/clusters/"+url.PathEscape(p.Name), nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return deleteClusterError(status), nil
	}
	return (*dto.DeleteClusterOK)(&v), nil
}

func (h *Handler) ReissueClusterToken(ctx context.Context, p dto.ReissueClusterTokenParams) (dto.ReissueClusterTokenRes, error) {
	v, status, err := h.invokeObject(ctx, "POST", "/api/v1/clusters/"+url.PathEscape(p.Name)+"/reissue-token", nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return reissueClusterError(status), nil
	}
	return (*dto.ReissueClusterTokenOK)(&v), nil
}

func (h *Handler) GetLatestInstallParams(ctx context.Context, p dto.GetLatestInstallParamsParams) (dto.GetLatestInstallParamsRes, error) {
	v, status, err := h.invokeObject(ctx, "GET", "/api/v1/clusters/"+url.PathEscape(p.Name)+"/latest-install-params", nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return latestInstallParamsError(status), nil
	}
	return (*dto.GetLatestInstallParamsOK)(&v), nil
}

func (h *Handler) CreateUninstallTask(ctx context.Context, p dto.CreateUninstallTaskParams) (dto.CreateUninstallTaskRes, error) {
	v, status, err := h.invokeObject(ctx, "POST", "/api/v1/clusters/"+url.PathEscape(p.Name)+"/uninstall", nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return createUninstallError(status), nil
	}
	return (*dto.CreateUninstallTaskOK)(&v), nil
}

func (h *Handler) ListUninstallTasks(ctx context.Context, p dto.ListUninstallTasksParams) (dto.ListUninstallTasksRes, error) {
	q := url.Values{}
	queryPage(q, p.Page, p.PageSize)
	path := "/api/v1/clusters/" + url.PathEscape(p.Name) + "/uninstall-tasks"
	if s := q.Encode(); s != "" {
		path += "?" + s
	}
	v, status, err := h.invokeObject(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return listUninstallError(status), nil
	}
	return (*dto.ListUninstallTasksOK)(&v), nil
}

func (h *Handler) RetryUninstallTask(ctx context.Context, p dto.RetryUninstallTaskParams) (dto.RetryUninstallTaskRes, error) {
	path := "/api/v1/clusters/" + url.PathEscape(p.Name) + "/uninstall-tasks/" + strconv.FormatInt(p.TaskID, 10) + "/retry"
	v, status, err := h.invokeObject(ctx, "POST", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return retryUninstallError(status), nil
	}
	return (*dto.RetryUninstallTaskOK)(&v), nil
}

func (h *Handler) ListInstallRecords(ctx context.Context, p dto.ListInstallRecordsParams) (dto.ListInstallRecordsRes, error) {
	q := url.Values{}
	queryPage(q, p.Page, p.PageSize)
	path := "/api/v1/install-records"
	if s := q.Encode(); s != "" {
		path += "?" + s
	}
	v, status, err := h.invokeObject(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return objectError(status), nil
	}
	return (*dto.ListInstallRecordsOK)(&v), nil
}
