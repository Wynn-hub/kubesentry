package handler

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/go-faster/jx"
)

func (h *Handler) platformInvoke(ctx context.Context, method, path string, body []byte) (map[string]jx.Raw, int, error) {
	if h.svcs.LegacyInvoke == nil {
		return nil, 503, nil
	}
	status, raw := h.svcs.LegacyInvoke(ctx, method, path, body)
	values := rawObject(raw)
	return values, status, nil
}

func platformResult(values map[string]jx.Raw, status int) (dto.GetPlatformOverviewRes, error) {
	if status == 400 {
		return &dto.GetPlatformOverviewBadRequest{Code: 40000, Message: "request failed"}, nil
	}
	if status >= 400 {
		return &dto.GetPlatformOverviewInternalServerError{Code: status * 100, Message: "request failed"}, nil
	}
	return (*dto.GetPlatformOverviewOK)(&values), nil
}

func (h *Handler) GetPlatformOverview(ctx context.Context, p dto.GetPlatformOverviewParams) (dto.GetPlatformOverviewRes, error) {
	q := url.Values{}
	if p.Page.Set {
		q.Set("page", strconv.Itoa(p.Page.Value))
	}
	if p.PageSize.Set {
		q.Set("pageSize", strconv.Itoa(int(p.PageSize.Value)))
	}
	if p.Health.Set {
		q.Set("health", p.Health.Value)
	}
	path := "/api/v1/platform/overview"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	values, status, err := h.platformInvoke(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	return platformResult(values, status)
}

func (h *Handler) GetPlatformInstanceTrend(ctx context.Context, p dto.GetPlatformInstanceTrendParams) (dto.GetPlatformInstanceTrendRes, error) {
	q := url.Values{}
	if p.Cluster.Set {
		q.Set("cluster", p.Cluster.Value)
	}
	if p.From.Set {
		q.Set("from", p.From.Value.Format("2006-01-02T15:04:05.999999999Z07:00"))
	}
	if p.To.Set {
		q.Set("to", p.To.Value.Format("2006-01-02T15:04:05.999999999Z07:00"))
	}
	path := "/api/v1/platform/instances/" + url.PathEscape(p.Component) + "/" + url.PathEscape(p.Instance) + "/trend"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	values, status, err := h.platformInvoke(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return &dto.GetPlatformInstanceTrendNotFound{Code: 40400, Message: "cluster not found"}, nil
	}
	if status >= 400 {
		if status == 400 {
			return &dto.GetPlatformInstanceTrendBadRequest{Code: 40000, Message: "request failed"}, nil
		}
		return &dto.GetPlatformInstanceTrendInternalServerError{Code: status * 100, Message: "request failed"}, nil
	}
	return (*dto.GetPlatformInstanceTrendOK)(&values), nil
}

func (h *Handler) ListPlatformLeadershipEvents(ctx context.Context, p dto.ListPlatformLeadershipEventsParams) (dto.ListPlatformLeadershipEventsRes, error) {
	q := url.Values{}
	if p.Page.Set {
		q.Set("page", strconv.Itoa(p.Page.Value))
	}
	if p.PageSize.Set {
		q.Set("pageSize", strconv.Itoa(int(p.PageSize.Value)))
	}
	if p.Component.Set {
		q.Set("component", p.Component.Value)
	}
	if p.Cluster.Set {
		q.Set("cluster", p.Cluster.Value)
	}
	if p.From.Set {
		q.Set("from", p.From.Value.Format("2006-01-02T15:04:05.999999999Z07:00"))
	}
	if p.To.Set {
		q.Set("to", p.To.Value.Format("2006-01-02T15:04:05.999999999Z07:00"))
	}
	path := "/api/v1/platform/leadership-events"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	values, status, err := h.platformInvoke(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return &dto.ListPlatformLeadershipEventsNotFound{Code: 40400, Message: "cluster not found"}, nil
	}
	if status >= 400 {
		if status == 400 {
			return &dto.ListPlatformLeadershipEventsBadRequest{Code: 40000, Message: "request failed"}, nil
		}
		return &dto.ListPlatformLeadershipEventsInternalServerError{Code: status * 100, Message: "request failed"}, nil
	}
	return (*dto.ListPlatformLeadershipEventsOK)(&values), nil
}

func (h *Handler) GetPlatformSettings(ctx context.Context) (dto.GetPlatformSettingsRes, error) {
	values, status, err := h.platformInvoke(ctx, "GET", "/api/v1/platform/settings", nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return &dto.ErrorResponse{Code: status * 100, Message: "request failed"}, nil
	}
	return (*dto.GetPlatformSettingsOK)(&values), nil
}

func (h *Handler) UpdatePlatformSettings(ctx context.Context, req *dto.UpdatePlatformSettingsReq) (dto.UpdatePlatformSettingsRes, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	values, status, err := h.platformInvoke(ctx, "PUT", "/api/v1/platform/settings", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		if status == 400 {
			return &dto.UpdatePlatformSettingsBadRequest{Code: 40000, Message: "request failed"}, nil
		}
		return &dto.UpdatePlatformSettingsInternalServerError{Code: status * 100, Message: "request failed"}, nil
	}
	return (*dto.UpdatePlatformSettingsOK)(&values), nil
}
