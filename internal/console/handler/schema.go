package handler

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
)

// @ID getFieldSchema
// @Summary 获取 Kubernetes 资源字段树
// @Description 从成员集群 OpenAPI schema 查询策略编辑器所需的资源字段树。
// @Tags Schema
// @Produce json
// @Param group query string false "API 组，核心组为空"
// @Param version query string true "API 版本"
// @Param resource query string true "资源复数名"
// @Param refresh query bool false "是否强制刷新缓存" default(false)
// @Success 200 {object} dto.SwagOptionalResponse{data=[]dto.FieldNodeView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 502 {object} dto.SwagErrorResponse
// @Router /schema/fields [get]
func (h *Handler) GetFieldSchema(ctx context.Context, p dto.GetFieldSchemaParams) (dto.GetFieldSchemaRes, error) {
	if h.svcs.Schema == nil {
		return &dto.GetFieldSchemaBadGateway{Code: 50200, Message: "schema discovery is not configured"}, nil
	}
	f, e := h.svcs.Schema(ctx, fieldSchemaOptions(p))
	if e != nil {
		return &dto.GetFieldSchemaBadGateway{Code: 50200, Message: e.Error()}, nil
	}
	data, err := dtoData[[]dto.FieldNode](f)
	if err != nil {
		return nil, err
	}
	return &dto.FieldSchemaResponse{Code: 0, Message: "success", Data: data}, nil
}

func fieldSchemaOptions(p dto.GetFieldSchemaParams) dto.FieldSchemaOptions {
	return dto.FieldSchemaOptions{Group: p.Group.Or(""), Version: p.Version, Resource: p.Resource, Refresh: p.Refresh.Or(false)}
}
