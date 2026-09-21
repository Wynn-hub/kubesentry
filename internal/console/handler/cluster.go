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

// @ID listClusters
// @Summary 分页查询集群
// @Description 查询平台已登记的集群及其在线状态和节点摘要。
// @Tags Cluster
// @Produce json
// @Param page query int false "页码" minimum(1) default(1)
// @Param pageSize query int false "每页记录数" Enums(10,20,50,100) default(20)
// @Param sortBy query string false "排序字段" Enums(name,phase,operatorVersion,lastHeartbeatAt,createdAt)
// @Param sortOrder query string false "排序方向" Enums(asc,desc) default(asc)
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.ClusterPageView}
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters [get]
func (h *Handler) ListClusters(ctx context.Context, p dto.ListClustersParams) (dto.ListClustersRes, error) {
	data, err := h.svcs.Cluster.List(ctx, p.Page.Or(1), int(p.PageSize.Or(dto.PageSize20)), string(p.SortBy.Or(dto.ListClustersSortByName)), string(p.SortOrder.Or(dto.ListClustersSortOrderAsc)))
	if err != nil {
		return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.ClusterListHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}

// @ID createCluster
// @Summary 创建集群登记
// @Description 创建集群登记记录并生成一次性 Helm 安装命令。
// @Tags Cluster
// @Accept json
// @Produce json
// @Param body body dto.ClusterCreateInput true "集群登记"
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.ClusterCommandView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters [post]
func (h *Handler) CreateCluster(ctx context.Context, req *dto.ClusterCreateRequest) (dto.CreateClusterRes, error) {
	var input dto.ClusterCreateInput
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.ClusterBadRequestHTTPError{Code: 40000, Message: err.Error()}, nil
	}
	name, command, err := h.svcs.Cluster.Create(ctx, input)
	if err != nil {
		return createClusterServiceError(err), nil
	}
	return &dto.ClusterCommandHTTPResponse{Code: 0, Message: "success", Data: dto.ClusterCommandView{Name: name, Command: command}}, nil
}

// @ID getCluster
// @Summary 查询集群详情
// @Description 查询指定集群的连接状态、版本和节点信息。
// @Tags Cluster
// @Produce json
// @Param name path string true "集群名称"
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.ClusterView}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name} [get]
func (h *Handler) GetCluster(ctx context.Context, p dto.GetClusterParams) (dto.GetClusterRes, error) {
	data, err := h.svcs.Cluster.Get(ctx, p.Name)
	if err != nil {
		return getClusterServiceError(err), nil
	}
	if data.Labels == nil {
		data.Labels = map[string]string{}
	}
	return &dto.ClusterDetailHTTPResponse{Code: 0, Message: "success", Data: *data}, nil
}

// @ID updateCluster
// @Summary 更新集群标签
// @Description 更新指定集群的控制台标签。
// @Tags Cluster
// @Accept json
// @Produce json
// @Param name path string true "集群名称"
// @Param body body dto.ClusterLabelsInput true "集群标签"
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.ClusterNameView}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name} [put]
func (h *Handler) UpdateCluster(ctx context.Context, req *dto.ClusterLabelsRequest, p dto.UpdateClusterParams) (dto.UpdateClusterRes, error) {
	var input dto.ClusterLabelsInput
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	if err := h.svcs.Cluster.UpdateLabels(ctx, p.Name, input); err != nil {
		return updateClusterServiceError(err), nil
	}
	return &dto.ClusterNameHTTPResponse{Code: 0, Message: "success", Data: dto.ClusterNameView{Name: p.Name}}, nil
}

// @ID deleteCluster
// @Summary 删除集群登记
// @Description 删除已卸载的集群登记；在线集群不能删除。
// @Tags Cluster
// @Produce json
// @Param name path string true "集群名称"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name} [delete]
func (h *Handler) DeleteCluster(ctx context.Context, p dto.DeleteClusterParams) (dto.DeleteClusterRes, error) {
	if err := h.svcs.Cluster.Delete(ctx, p.Name); err != nil {
		return deleteClusterServiceError(err), nil
	}
	return &dto.ClusterEmptyHTTPResponse{Code: 0, Message: "success"}, nil
}

// @ID reissueClusterToken
// @Summary 重新签发集群令牌
// @Description 为尚未完成注册的集群重新签发一次性安装令牌。
// @Tags Cluster
// @Accept json
// @Produce json
// @Param name path string true "集群名称"
// @Param body body dto.ClusterTokenInput true "安装参数"
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.ClusterCommandView}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name}/reissue-token [post]
func (h *Handler) ReissueClusterToken(ctx context.Context, req *dto.ClusterTokenRequest, p dto.ReissueClusterTokenParams) (dto.ReissueClusterTokenRes, error) {
	var input dto.ClusterTokenInput
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	name, command, err := h.svcs.Cluster.ReissueToken(ctx, p.Name, input)
	if err != nil {
		return reissueClusterServiceError(err), nil
	}
	return &dto.ClusterCommandHTTPResponse{Code: 0, Message: "success", Data: dto.ClusterCommandView{Name: name, Command: command}}, nil
}

// @ID getLatestInstallParams
// @Summary 查询最新安装参数
// @Description 查询指定集群最近一次生成的非敏感安装参数。
// @Tags Cluster
// @Produce json
// @Param name path string true "集群名称"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.ClusterInstallInput}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name}/latest-install-params [get]
func (h *Handler) GetLatestInstallParams(ctx context.Context, p dto.GetLatestInstallParamsParams) (dto.GetLatestInstallParamsRes, error) {
	data, err := h.svcs.Cluster.LatestInstallParams(ctx, p.Name)
	if err != nil {
		return &dto.ClusterTaskInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	if data == nil {
		return &dto.InstallParamsHTTPResponse{Code: 0, Message: "success"}, nil
	}
	return &dto.InstallParamsHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}

// @ID createUninstallTask
// @Summary 创建集群卸载任务
// @Description 向在线集群下发卸载其成员组件的持久化任务。
// @Tags Cluster
// @Produce json
// @Param name path string true "集群名称"
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.UninstallTaskHTTPView}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name}/uninstall [post]
func (h *Handler) CreateUninstallTask(ctx context.Context, p dto.CreateUninstallTaskParams) (dto.CreateUninstallTaskRes, error) {
	data, err := h.svcs.Cluster.CreateUninstall(ctx, p.Name)
	if err != nil {
		return createUninstallServiceError(err), nil
	}
	return &dto.UninstallTaskHTTPResponse{Code: 0, Message: "success", Data: uninstallTask(data)}, nil
}

// @ID listUninstallTasks
// @Summary 查询集群卸载任务
// @Description 查询指定集群的全部卸载任务及失败原因。
// @Tags Cluster
// @Produce json
// @Param name path string true "集群名称"
// @Success 200 {object} dto.SwagSuccessResponse{data=[]dto.UninstallTaskHTTPView}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name}/uninstall-tasks [get]
func (h *Handler) ListUninstallTasks(ctx context.Context, p dto.ListUninstallTasksParams) (dto.ListUninstallTasksRes, error) {
	data, err := h.svcs.Cluster.ListUninstall(ctx, p.Name)
	if err != nil {
		return listUninstallServiceError(err), nil
	}
	items := make([]dto.UninstallTaskHTTPView, 0, len(data))
	for i := range data {
		items = append(items, uninstallTask(&data[i]))
	}
	return &dto.UninstallTaskListHTTPResponse{Code: 0, Message: "success", Data: items}, nil
}

// @ID retryUninstallTask
// @Summary 重试集群卸载任务
// @Description 重新投递失败的集群卸载任务。
// @Tags Cluster
// @Produce json
// @Param name path string true "集群名称"
// @Param taskID path dto.RetryUninstallTaskPath true "卸载任务 ID"
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.UninstallTaskIDView}
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /clusters/{name}/uninstall-tasks/{taskID}/retry [post]
func (h *Handler) RetryUninstallTask(ctx context.Context, p dto.RetryUninstallTaskParams) (dto.RetryUninstallTaskRes, error) {
	taskID, err := strconv.ParseInt(string(p.TaskID), 10, 64)
	if err != nil || taskID <= 0 {
		return &dto.ClusterTaskNotFoundHTTPError{Code: 40400, Message: "task not found"}, nil
	}
	if err := h.svcs.Cluster.RetryUninstall(ctx, p.Name, taskID); err != nil {
		return retryUninstallServiceError(err), nil
	}
	return &dto.UninstallTaskIDHTTPResponse{Code: 0, Message: "success", Data: dto.UninstallTaskIDView{ID: string(p.TaskID)}}, nil
}

// @ID listInstallRecords
// @Summary 查询安装记录
// @Description 分页查询平台生成的集群安装参数记录。
// @Tags Cluster
// @Produce json
// @Param page query int false "页码" minimum(1) default(1)
// @Param pageSize query int false "每页记录数" Enums(10,20,50,100) default(20)
// @Param sortBy query string false "排序字段" Enums(clusterName,namespace,createdAt)
// @Param sortOrder query string false "排序方向" Enums(asc,desc) default(desc)
// @Success 200 {object} dto.SwagSuccessResponse{data=dto.InstallRecordPageView}
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /install-records [get]
func (h *Handler) ListInstallRecords(ctx context.Context, p dto.ListInstallRecordsParams) (dto.ListInstallRecordsRes, error) {
	data, err := h.svcs.Cluster.ListInstallRecords(ctx, p.Page.Or(1), int(p.PageSize.Or(dto.PageSize20)), string(p.SortBy.Or(dto.ListInstallRecordsSortByCreatedAt)), string(p.SortOrder.Or(dto.ListInstallRecordsSortOrderDesc)))
	if err != nil {
		return &dto.ClusterTaskInternalServerHTTPError{Code: 50000, Message: "request failed"}, nil
	}
	return &dto.InstallRecordListHTTPResponse{Code: 0, Message: "success", Data: data}, nil
}

func uninstallTask(item *dto.UninstallTaskView) dto.UninstallTaskHTTPView {
	result := dto.UninstallTaskHTTPView{
		ID:          strconv.FormatInt(item.ID, 10),
		Status:      item.Status,
		RequestedAt: item.RequestedAt,
	}
	if item.Error != "" {
		result.Error = item.Error
	}
	if item.CompletedAt != nil {
		result.CompletedAt = item.CompletedAt
	}
	return result
}

func createClusterServiceError(err error) dto.CreateClusterRes {
	if service.IsConflict(err) {
		return &dto.ClusterConflictHTTPError{Code: 40900, Message: err.Error()}
	}
	if _, ok := err.(*service.ValidationError); ok {
		return &dto.ClusterBadRequestHTTPError{Code: 40000, Message: err.Error()}
	}
	return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}
}

func getClusterServiceError(err error) dto.GetClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ClusterNotFoundHTTPError{Code: 40400, Message: "cluster not found"}
	}
	return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}
}

func updateClusterServiceError(err error) dto.UpdateClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ClusterNotFoundHTTPError{Code: 40400, Message: "cluster not found"}
	}
	return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}
}

func deleteClusterServiceError(err error) dto.DeleteClusterRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ClusterNotFoundHTTPError{Code: 40400, Message: "cluster not found"}
	}
	if service.IsConflict(err) {
		return &dto.ClusterConflictHTTPError{Code: 40900, Message: err.Error()}
	}
	return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}
}

func reissueClusterServiceError(err error) dto.ReissueClusterTokenRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ClusterNotFoundHTTPError{Code: 40400, Message: "cluster not found"}
	}
	if service.IsConflict(err) {
		return &dto.ClusterConflictHTTPError{Code: 40900, Message: err.Error()}
	}
	return &dto.ClusterInternalServerHTTPError{Code: 50000, Message: "request failed"}
}

func createUninstallServiceError(err error) dto.CreateUninstallTaskRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ClusterTaskNotFoundHTTPError{Code: 40400, Message: "cluster not found"}
	}
	if service.IsConflict(err) {
		return &dto.ClusterTaskConflictHTTPError{Code: 40900, Message: err.Error()}
	}
	return &dto.ClusterTaskInternalServerHTTPError{Code: 50000, Message: "request failed"}
}

func listUninstallServiceError(err error) dto.ListUninstallTasksRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ClusterTaskNotFoundHTTPError{Code: 40400, Message: "cluster not found"}
	}
	return &dto.ClusterTaskInternalServerHTTPError{Code: 50000, Message: "request failed"}
}

func retryUninstallServiceError(err error) dto.RetryUninstallTaskRes {
	if errors.Is(err, service.ErrNotFound) {
		return &dto.ClusterTaskNotFoundHTTPError{Code: 40400, Message: "task not found"}
	}
	if service.IsConflict(err) {
		return &dto.ClusterTaskConflictHTTPError{Code: 40900, Message: err.Error()}
	}
	return &dto.ClusterTaskInternalServerHTTPError{Code: 50000, Message: "request failed"}
}
