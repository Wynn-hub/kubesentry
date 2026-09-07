package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/service"
)

func dtoData[T any](data any) (T, error) {
	var value T
	b, err := json.Marshal(data)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(b, &value); err != nil {
		return value, err
	}
	return value, nil
}

func (h *Handler) ListClusters(ctx context.Context, p dto.ListClustersParams) (dto.ListClustersRes, error) {
	data, err := h.svcs.Cluster.List(ctx, p.Page.Or(1), int(p.PageSize.Or(dto.PageSize20)), string(p.SortBy.Or(dto.ListClustersSortByName)), string(p.SortOrder.Or(dto.ListClustersSortOrderAsc)))
	if err != nil {
		return objectError(500), nil
	}
	v, err := dtoData[dto.ClusterListResponseData](data)
	if err != nil {
		return nil, err
	}
	return &dto.ClusterListResponse{Code: 0, Message: "success", Data: v}, nil
}

func (h *Handler) CreateCluster(ctx context.Context, req *dto.ClusterCreateRequest) (dto.CreateClusterRes, error) {
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
	return &dto.ClusterCommandResponse{Code: 0, Message: "success", Data: dto.ClusterCommand{Name: name, Command: command}}, nil
}

func (h *Handler) GetCluster(ctx context.Context, p dto.GetClusterParams) (dto.GetClusterRes, error) {
	data, err := h.svcs.Cluster.Get(ctx, p.Name)
	if err != nil {
		return getClusterServiceError(err), nil
	}
	if data.Labels == nil {
		data.Labels = map[string]string{}
	}
	v, err := dtoData[dto.ClusterDetail](data)
	if err != nil {
		return nil, err
	}
	return &dto.ClusterDetailResponse{Code: 0, Message: "success", Data: v}, nil
}

func (h *Handler) UpdateCluster(ctx context.Context, req *dto.ClusterLabelsRequest, p dto.UpdateClusterParams) (dto.UpdateClusterRes, error) {
	var input service.UpdateClusterLabelsRequest
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.UpdateClusterInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	if err := h.svcs.Cluster.UpdateLabels(ctx, p.Name, input); err != nil {
		return updateClusterServiceError(err), nil
	}
	return &dto.ClusterNameResponse{Code: 0, Message: "success", Data: dto.ClusterNameData{Name: p.Name}}, nil
}

func (h *Handler) DeleteCluster(ctx context.Context, p dto.DeleteClusterParams) (dto.DeleteClusterRes, error) {
	if err := h.svcs.Cluster.Delete(ctx, p.Name); err != nil {
		return deleteClusterServiceError(err), nil
	}
	return &dto.EmptyResponse{Code: 0, Message: "success"}, nil
}

func (h *Handler) ReissueClusterToken(ctx context.Context, req *dto.ClusterTokenRequest, p dto.ReissueClusterTokenParams) (dto.ReissueClusterTokenRes, error) {
	var input service.ReissueClusterTokenRequest
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.ReissueClusterTokenInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	name, command, err := h.svcs.Cluster.ReissueToken(ctx, p.Name, input)
	if err != nil {
		return reissueClusterServiceError(err), nil
	}
	return &dto.ClusterCommandResponse{Code: 0, Message: "success", Data: dto.ClusterCommand{Name: name, Command: command}}, nil
}

func (h *Handler) GetLatestInstallParams(ctx context.Context, p dto.GetLatestInstallParamsParams) (dto.GetLatestInstallParamsRes, error) {
	data, err := h.svcs.Cluster.LatestInstallParams(ctx, p.Name)
	if err != nil {
		return &dto.GetLatestInstallParamsInternalServerError{Code: 50000, Message: "request failed"}, nil
	}
	if data == nil {
		return &dto.InstallParamsResponse{Code: 0, Message: "success"}, nil
	}
	v, err := dtoData[dto.InstallParams](data)
	if err != nil {
		return nil, err
	}
	return &dto.InstallParamsResponse{Code: 0, Message: "success", Data: dto.NewOptInstallParams(v)}, nil
}

func (h *Handler) CreateUninstallTask(ctx context.Context, p dto.CreateUninstallTaskParams) (dto.CreateUninstallTaskRes, error) {
	data, err := h.svcs.Cluster.CreateUninstall(ctx, p.Name)
	if err != nil {
		return createUninstallServiceError(err), nil
	}
	return &dto.UninstallTaskResponse{Code: 0, Message: "success", Data: uninstallTask(data)}, nil
}

func (h *Handler) ListUninstallTasks(ctx context.Context, p dto.ListUninstallTasksParams) (dto.ListUninstallTasksRes, error) {
	data, err := h.svcs.Cluster.ListUninstall(ctx, p.Name)
	if err != nil {
		return listUninstallServiceError(err), nil
	}
	items := make([]dto.UninstallTask, 0, len(data))
	for i := range data {
		items = append(items, uninstallTask(&data[i]))
	}
	return &dto.UninstallTaskListResponse{Code: 0, Message: "success", Data: items}, nil
}

func (h *Handler) RetryUninstallTask(ctx context.Context, p dto.RetryUninstallTaskParams) (dto.RetryUninstallTaskRes, error) {
	taskID, err := strconv.ParseInt(string(p.TaskID), 10, 64)
	if err != nil || taskID <= 0 {
		return &dto.RetryUninstallTaskNotFound{Code: 40400, Message: "task not found"}, nil
	}
	if err := h.svcs.Cluster.RetryUninstall(ctx, p.Name, taskID); err != nil {
		return retryUninstallServiceError(err), nil
	}
	return &dto.UninstallTaskIDResponse{Code: 0, Message: "success", Data: dto.UninstallTaskIDData{ID: p.TaskID}}, nil
}

func (h *Handler) ListInstallRecords(ctx context.Context, p dto.ListInstallRecordsParams) (dto.ListInstallRecordsRes, error) {
	data, err := h.svcs.Cluster.ListInstallRecords(ctx, p.Page.Or(1), int(p.PageSize.Or(dto.PageSize20)), string(p.SortBy.Or(dto.ListInstallRecordsSortByCreatedAt)), string(p.SortOrder.Or(dto.ListInstallRecordsSortOrderDesc)))
	if err != nil {
		return objectError(500), nil
	}
	v, err := dtoData[dto.InstallRecordListResponseData](data)
	if err != nil {
		return nil, err
	}
	return &dto.InstallRecordListResponse{Code: 0, Message: "success", Data: v}, nil
}

func uninstallTask(item *service.UninstallTaskItem) dto.UninstallTask {
	result := dto.UninstallTask{
		ID:          dto.ResourceID(strconv.FormatInt(item.ID, 10)),
		Status:      dto.UninstallTaskStatus(item.Status),
		RequestedAt: item.RequestedAt,
	}
	if item.Error != "" {
		result.Error = dto.NewOptString(item.Error)
	}
	if item.CompletedAt != nil {
		result.CompletedAt = dto.NewOptDateTime(*item.CompletedAt)
	}
	return result
}

func createClusterServiceError(err error) dto.CreateClusterRes {
	if service.IsConflict(err) {
		return &dto.CreateClusterConflict{Code: 40900, Message: err.Error()}
	}
	if _, ok := err.(*service.ValidationError); ok {
		return &dto.CreateClusterBadRequest{Code: 40000, Message: err.Error()}
	}
	return &dto.CreateClusterInternalServerError{Code: 50000, Message: "request failed"}
}

func getClusterServiceError(err error) dto.GetClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.GetClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.GetClusterInternalServerError{Code: 50000, Message: "request failed"}
}

func updateClusterServiceError(err error) dto.UpdateClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.UpdateClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.UpdateClusterInternalServerError{Code: 50000, Message: "request failed"}
}

func deleteClusterServiceError(err error) dto.DeleteClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.DeleteClusterNotFound{Code: 40400, Message: "cluster not found"}
	}
	if service.IsConflict(err) {
		return &dto.DeleteClusterConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.DeleteClusterInternalServerError{Code: 50000, Message: "request failed"}
}

func reissueClusterServiceError(err error) dto.ReissueClusterTokenRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ReissueClusterTokenNotFound{Code: 40400, Message: "cluster not found"}
	}
	if service.IsConflict(err) {
		return &dto.ReissueClusterTokenConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.ReissueClusterTokenInternalServerError{Code: 50000, Message: "request failed"}
}

func createUninstallServiceError(err error) dto.CreateUninstallTaskRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.CreateUninstallTaskNotFound{Code: 40400, Message: "cluster not found"}
	}
	if service.IsConflict(err) {
		return &dto.CreateUninstallTaskConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.CreateUninstallTaskInternalServerError{Code: 50000, Message: "request failed"}
}

func listUninstallServiceError(err error) dto.ListUninstallTasksRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ListUninstallTasksNotFound{Code: 40400, Message: "cluster not found"}
	}
	return &dto.ListUninstallTasksInternalServerError{Code: 50000, Message: "request failed"}
}

func retryUninstallServiceError(err error) dto.RetryUninstallTaskRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.RetryUninstallTaskNotFound{Code: 40400, Message: "task not found"}
	}
	if service.IsConflict(err) {
		return &dto.RetryUninstallTaskConflict{Code: 40900, Message: err.Error()}
	}
	return &dto.RetryUninstallTaskInternalServerError{Code: 50000, Message: "request failed"}
}
