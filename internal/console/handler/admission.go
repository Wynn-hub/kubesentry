package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/go-faster/jx"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func admissionItem(e dbrepo.AdmissionEvent) dto.AdmissionEventItem {
	violations := []string{}
	_ = json.Unmarshal(e.Violations, &violations)
	return dto.AdmissionEventItem{
		ID: dto.ResourceID(strconv.FormatInt(e.ID, 10)), EventId: e.EventID, ClusterName: e.ClusterName, OccurredAt: e.OccurredAt,
		Verdict: dto.AdmissionEventItemVerdict(e.Verdict), ApiGroup: e.APIGroup, ApiVersion: e.APIVersion, Resource: e.Resource, Operation: e.Operation,
		Namespace: e.Namespace, ResourceName: e.ResourceName, Username: e.Username, PolicyNames: []string(e.PolicyNames), Violations: violations,
	}
}

func rawObject(raw []byte) map[string]jx.Raw {
	value := map[string]jx.Raw{}
	if len(raw) == 0 || string(raw) == "null" {
		return value
	}
	var decoded map[string]json.RawMessage
	if json.Unmarshal(raw, &decoded) != nil {
		return value
	}
	for key, item := range decoded {
		value[key] = jx.Raw(item)
	}
	return value
}

func admissionDetail(e dbrepo.AdmissionEvent) dto.AdmissionEventDetail {
	base := admissionItem(e)
	return dto.AdmissionEventDetail{ID: base.ID, EventId: base.EventId, ClusterName: base.ClusterName, OccurredAt: base.OccurredAt, Verdict: dto.AdmissionEventDetailVerdict(e.Verdict), ApiGroup: base.ApiGroup, ApiVersion: base.ApiVersion, Resource: base.Resource, Operation: base.Operation, Namespace: base.Namespace, ResourceName: base.ResourceName, Username: base.Username, PolicyNames: base.PolicyNames, Violations: base.Violations, RequestUid: e.RequestUID, UserGroups: []string(e.UserGroups), PolicyGroupNames: []string(e.PolicyGroupNames), MetadataSnapshot: dto.AdmissionEventDetailMetadataSnapshot(rawObject(e.MetadataSnapshot)), SpecSnapshot: dto.AdmissionEventDetailSpecSnapshot(rawObject(e.SpecSnapshot)), StoragePolicyVersion: e.StoragePolicyVersion}
}

func (h *Handler) ListAdmissionEvents(ctx context.Context, params dto.ListAdmissionEventsParams) (dto.ListAdmissionEventsRes, error) {
	page := params.Page.Or(1)
	if page < 1 {
		return &dto.ListAdmissionEventsBadRequest{Code: 40000, Message: "invalid page: must be greater than zero"}, nil
	}
	pageSize := int(params.PageSize.Or(dto.PageSize20))
	sortBy := string(params.SortBy.Or(dto.ListAdmissionEventsSortByOccurredAt))
	sortOrder := string(params.SortOrder.Or(dto.ListAdmissionEventsSortOrderDesc))
	opts := dbrepo.AdmissionEventPageOptions{PageOptions: dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: sortBy, Desc: sortOrder == "desc"}, ClusterNames: csv(params.Cluster.Or("")), PolicyName: params.Policy.Or(""), Namespaces: csv(params.Namespace.Or("")), GVKs: csv(params.Gvk.Or("")), ResourceName: params.ResourceName.Or(""), Username: params.Username.Or("")}
	if v := params.Verdict; v.Set {
		opts.Verdicts = []string{string(v.Value)}
	}
	if v := params.From; v.Set {
		opts.From = &v.Value
	}
	if v := params.To; v.Set {
		opts.To = &v.Value
	}
	items, total, err := h.svcs.Admission.List(ctx, opts)
	if err != nil {
		return &dto.ListAdmissionEventsInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	out := make([]dto.AdmissionEventItem, 0, len(items))
	for _, item := range items {
		out = append(out, admissionItem(item))
	}
	return &dto.AdmissionEventListResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventListData{Set: true, Value: dto.AdmissionEventListData{Total: int(total), Page: page, PageSize: dto.AdmissionEventListDataPageSize(pageSize), Items: out}}}, nil
}

func (h *Handler) GetAdmissionEvent(ctx context.Context, params dto.GetAdmissionEventParams) (dto.GetAdmissionEventRes, error) {
	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		return &dto.GetAdmissionEventBadRequest{Code: 40000, Message: "invalid event id"}, nil
	}
	e, err := h.svcs.Admission.Get(ctx, id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &dto.GetAdmissionEventNotFound{Code: 40400, Message: "admission event not found"}, nil
		}
		return &dto.GetAdmissionEventInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	d := admissionDetail(*e)
	return &dto.AdmissionEventDetailResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventDetail{Set: true, Value: d}}, nil
}

func (h *Handler) GetAdmissionEventTrend(ctx context.Context, params dto.GetAdmissionEventTrendParams) (dto.GetAdmissionEventTrendRes, error) {
	if params.To.Before(params.From) {
		return &dto.GetAdmissionEventTrendBadRequest{Code: 40000, Message: "to must be after from"}, nil
	}
	rows, err := h.svcs.Admission.Trend(ctx, params.From, params.To)
	if err != nil {
		return &dto.GetAdmissionEventTrendInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	out := make([]dto.AdmissionEventTrendItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.AdmissionEventTrendItem{Bucket: row.Bucket, Verdict: dto.AdmissionEventTrendItemVerdict(row.Verdict), Count: row.Count})
	}
	return &dto.AdmissionEventTrendResponse{Code: 0, Message: "success", Data: out}, nil
}

func admissionSettings(s *dbrepo.AdmissionEventSettings, policies []dbrepo.AdmissionStoragePolicy) dto.AdmissionEventSettings {
	out := make([]dto.AdmissionStoragePolicy, 0, len(policies))
	for _, p := range policies {
		verdicts := make([]dto.AdmissionStoragePolicyVerdictsItem, 0, len(p.Verdicts))
		for _, v := range p.Verdicts {
			verdicts = append(verdicts, dto.AdmissionStoragePolicyVerdictsItem(v))
		}
		out = append(out, dto.AdmissionStoragePolicy{MatchMode: dto.AdmissionStoragePolicyMatchMode(p.MatchMode), Verdicts: verdicts, Clusters: []string(p.Clusters), Gvks: []string(p.GVKs), Namespaces: []string(p.Namespaces)})
	}
	return dto.AdmissionEventSettings{DefaultRetentionDays: s.DefaultRetentionDays, RetryEnabled: s.RetryEnabled, MaxRetryAttempts: s.MaxRetryAttempts, QueueCapacity: s.QueueCapacity, IncidentRetentionDays: s.IncidentRetentionDays, ObservationEnabled: s.ObservationEnabled, StoragePolicyVersion: s.StoragePolicyVersion, StoragePolicies: out}
}

func (h *Handler) GetAdmissionEventSettings(ctx context.Context) (dto.GetAdmissionEventSettingsRes, error) {
	s, p, err := h.svcs.Admission.Settings(ctx)
	if err != nil {
		return &dto.GetAdmissionEventSettingsInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	return &dto.AdmissionEventSettingsResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventSettings{Set: true, Value: admissionSettings(s, p)}}, nil
}

func (h *Handler) UpdateAdmissionEventSettings(ctx context.Context, req *dto.AdmissionEventSettingsRequest) (dto.UpdateAdmissionEventSettingsRes, error) {
	maxRetryAttempts := 3
	if req.MaxRetryAttempts.Set {
		maxRetryAttempts = req.MaxRetryAttempts.Value
	}
	queueCapacity := 1024
	if req.QueueCapacity.Set {
		queueCapacity = req.QueueCapacity.Value
	}
	incidentRetentionDays := 7
	if req.IncidentRetentionDays.Set {
		incidentRetentionDays = req.IncidentRetentionDays.Value
	}
	retryEnabled := false
	if req.RetryEnabled.Set {
		retryEnabled = req.RetryEnabled.Value
	}
	observationEnabled := false
	if req.ObservationEnabled.Set {
		observationEnabled = req.ObservationEnabled.Value
	}
	policies := make([]dbrepo.AdmissionStoragePolicy, 0, len(req.StoragePolicies))
	for i, p := range req.StoragePolicies {
		verdicts := make([]string, 0, len(p.Verdicts))
		for _, v := range p.Verdicts {
			verdicts = append(verdicts, string(v))
		}
		policies = append(policies, dbrepo.AdmissionStoragePolicy{Priority: i + 1, MatchMode: string(p.MatchMode), Verdicts: verdicts, Clusters: p.Clusters, GVKs: p.Gvks, Namespaces: p.Namespaces})
	}
	if err := h.svcs.Admission.UpdateSettings(ctx, &dbrepo.AdmissionEventSettings{ID: 1, DefaultRetentionDays: req.DefaultRetentionDays, RetryEnabled: retryEnabled, MaxRetryAttempts: maxRetryAttempts, QueueCapacity: queueCapacity, IncidentRetentionDays: incidentRetentionDays, ObservationEnabled: observationEnabled}, policies); err != nil {
		return &dto.UpdateAdmissionEventSettingsInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	s, p, err := h.svcs.Admission.Settings(ctx)
	if err != nil {
		return &dto.UpdateAdmissionEventSettingsInternalServerError{Code: 50000, Message: err.Error()}, nil
	}
	return &dto.AdmissionEventSettingsResponse{Code: 0, Message: "success", Data: dto.OptAdmissionEventSettings{Set: true, Value: admissionSettings(s, p)}}, nil
}

func csv(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
