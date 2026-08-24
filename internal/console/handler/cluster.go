package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-faster/jx"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func rawSuccess(data any) (map[string]jx.Raw, error) {
	response := map[string]any{"code": 0, "message": "success"}
	if data != nil {
		response["data"] = data
	}
	b, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return rawObject(b), nil
}

func (h *Handler) ListClusters(ctx context.Context, p dto.ListClustersParams) (dto.ListClustersRes, error) {
	data, err := h.svcs.Cluster.List(ctx, p.Page.Or(1), int(p.PageSize.Or(dto.PageSize20)), string(p.SortBy.Or(dto.ListClustersSortByName)), string(p.SortOrder.Or(dto.ListClustersSortOrderAsc)))
	if err != nil {
		return objectError(500), nil
	}
	v, err := rawSuccess(data)
	if err != nil {
		return nil, err
	}
	return (*dto.ListClustersOK)(&v), nil
}

func (h *Handler) CreateCluster(ctx context.Context, req dto.CreateClusterReq) (dto.CreateClusterRes, error) {
	var input service.CreateClusterRequest
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.CreateClusterBadRequest{Code: 40000, Message: err.Error()}, nil
	}
	name, command, err := h.svcs.Cluster.Create(ctx, input)
	if err != nil {
		return createClusterServiceError(err), nil
	}
	v, err := rawSuccess(map[string]string{"name": name, "command": command})
	if err != nil {
		return nil, err
	}
	return (*dto.CreateClusterOK)(&v), nil
}

func (h *Handler) GetCluster(ctx context.Context, p dto.GetClusterParams) (dto.GetClusterRes, error) {
	data, err := h.svcs.Cluster.Get(ctx, p.Name)
	if err != nil {
		return getClusterServiceError(err), nil
	}
	v, err := rawSuccess(data)
	if err != nil {
		return nil, err
	}
	return (*dto.GetClusterOK)(&v), nil
}

func (h *Handler) UpdateCluster(ctx context.Context, req dto.UpdateClusterReq, p dto.UpdateClusterParams) (dto.UpdateClusterRes, error) {
	var input service.UpdateClusterLabelsRequest
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.UpdateClusterInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	if err := h.svcs.Cluster.UpdateLabels(ctx, p.Name, input); err != nil {
		return updateClusterServiceError(err), nil
	}
	v, err := rawSuccess(map[string]string{"name": p.Name})
	if err != nil {
		return nil, err
	}
	return (*dto.UpdateClusterOK)(&v), nil
}

func (h *Handler) DeleteCluster(ctx context.Context, p dto.DeleteClusterParams) (dto.DeleteClusterRes, error) {
	if err := h.svcs.Cluster.Delete(ctx, p.Name); err != nil {
		return deleteClusterServiceError(err), nil
	}
	v, err := rawSuccess(nil)
	if err != nil {
		return nil, err
	}
	return (*dto.DeleteClusterOK)(&v), nil
}

func (h *Handler) ReissueClusterToken(ctx context.Context, req dto.ReissueClusterTokenReq, p dto.ReissueClusterTokenParams) (dto.ReissueClusterTokenRes, error) {
	var input service.ReissueClusterTokenRequest
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.ReissueClusterTokenInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	name, command, err := h.svcs.Cluster.ReissueToken(ctx, p.Name, input)
	if err != nil {
		return reissueClusterServiceError(err), nil
	}
	v, err := rawSuccess(map[string]string{"name": name, "command": command})
	if err != nil {
		return nil, err
	}
	return (*dto.ReissueClusterTokenOK)(&v), nil
}

func (h *Handler) GetLatestInstallParams(ctx context.Context, p dto.GetLatestInstallParamsParams) (dto.GetLatestInstallParamsRes, error) {
	data, err := h.svcs.Cluster.LatestInstallParams(ctx, p.Name)
	if err != nil {
		return &dto.GetLatestInstallParamsInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	if data == nil {
		v, err := rawSuccess(nil)
		if err != nil {
			return nil, err
		}
		return (*dto.GetLatestInstallParamsOK)(&v), nil
	}
	v, err := rawSuccess(data)
	if err != nil {
		return nil, err
	}
	return (*dto.GetLatestInstallParamsOK)(&v), nil
}

func (h *Handler) CreateUninstallTask(ctx context.Context, p dto.CreateUninstallTaskParams) (dto.CreateUninstallTaskRes, error) {
	data, err := h.svcs.Cluster.CreateUninstall(ctx, p.Name)
	if err != nil {
		return createUninstallServiceError(err), nil
	}
	v, err := rawSuccess(data)
	if err != nil {
		return nil, err
	}
	return (*dto.CreateUninstallTaskOK)(&v), nil
}

func (h *Handler) ListUninstallTasks(ctx context.Context, p dto.ListUninstallTasksParams) (dto.ListUninstallTasksRes, error) {
	data, err := h.svcs.Cluster.ListUninstall(ctx, p.Name)
	if err != nil {
		return listUninstallServiceError(err), nil
	}
	v, err := rawSuccess(data)
	if err != nil {
		return nil, err
	}
	return (*dto.ListUninstallTasksOK)(&v), nil
}

func (h *Handler) RetryUninstallTask(ctx context.Context, p dto.RetryUninstallTaskParams) (dto.RetryUninstallTaskRes, error) {
	if err := h.svcs.Cluster.RetryUninstall(ctx, p.Name, p.TaskID); err != nil {
		return retryUninstallServiceError(err), nil
	}
	v, err := rawSuccess(map[string]int64{"id": p.TaskID})
	if err != nil {
		return nil, err
	}
	return (*dto.RetryUninstallTaskOK)(&v), nil
}

func (h *Handler) ListInstallRecords(ctx context.Context, p dto.ListInstallRecordsParams) (dto.ListInstallRecordsRes, error) {
	data, err := h.svcs.Cluster.ListInstallRecords(ctx, p.Page.Or(1), int(p.PageSize.Or(dto.PageSize20)), string(p.SortBy.Or(dto.ListInstallRecordsSortByCreatedAt)), string(p.SortOrder.Or(dto.ListInstallRecordsSortOrderDesc)))
	if err != nil {
		return objectError(500), nil
	}
	v, err := rawSuccess(data)
	if err != nil {
		return nil, err
	}
	return (*dto.ListInstallRecordsOK)(&v), nil
}

func createClusterServiceError(err error) dto.CreateClusterRes {
	if errors.Is(err, dbrepo.ErrConflict) {
		return &dto.CreateClusterConflict{Code: 40900, Message: err.Error()}
	}
	if _, ok := err.(*service.ValidationError); ok {
		return &dto.CreateClusterBadRequest{Code: 40000, Message: err.Error()}
	}
	return &dto.CreateClusterInternalServerError{Code: 50000, Message: err.Error()}
}

func getClusterServiceError(err error) dto.GetClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.GetClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.GetClusterInternalServerError{Code: 50000, Message: err.Error()}
}

func updateClusterServiceError(err error) dto.UpdateClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.UpdateClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.UpdateClusterInternalServerError{Code: 50000, Message: err.Error()}
}

func deleteClusterServiceError(err error) dto.DeleteClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.DeleteClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	if errors.Is(err, dbrepo.ErrConflict) {
		return &dto.DeleteClusterConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.DeleteClusterInternalServerError{Code: 50000, Message: err.Error()}
}

func reissueClusterServiceError(err error) dto.ReissueClusterTokenRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ReissueClusterTokenNotFound{Code: 40400, Message: "cluster not found"}
	}
	if errors.Is(err, dbrepo.ErrConflict) {
		return &dto.ReissueClusterTokenConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.ReissueClusterTokenInternalServerError{Code: 50000, Message: err.Error()}
}

func createUninstallServiceError(err error) dto.CreateUninstallTaskRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.CreateUninstallTaskNotFound{Code: 40400, Message: "cluster not found"}
	}
	if errors.Is(err, dbrepo.ErrConflict) {
		return &dto.CreateUninstallTaskConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.CreateUninstallTaskInternalServerError{Code: 50000, Message: err.Error()}
}

func listUninstallServiceError(err error) dto.ListUninstallTasksRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ListUninstallTasksNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.ListUninstallTasksInternalServerError{Code: 50000, Message: err.Error()}
}

func retryUninstallServiceError(err error) dto.RetryUninstallTaskRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.RetryUninstallTaskNotFound{Code: 40400, Message: "task not found"}
	}
	if errors.Is(err, dbrepo.ErrConflict) {
		return &dto.RetryUninstallTaskConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.RetryUninstallTaskInternalServerError{Code: 50000, Message: err.Error()}
}
