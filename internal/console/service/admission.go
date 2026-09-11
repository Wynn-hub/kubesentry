package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/jx"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

type AdmissionService struct {
	repo     repository.AdmissionRepository
	notifier Notifier
}

func NewAdmissionService(repo repository.AdmissionRepository, notifier Notifier) *AdmissionService {
	return &AdmissionService{repo: repo, notifier: notifier}
}

func (s *AdmissionService) Settings(ctx context.Context) (*dto.AdmissionEventSettings, error) {
	settings, err := s.repo.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	policies, err := s.repo.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	result := admissionSettings(settings, policies)
	return &result, nil
}

func (s *AdmissionService) UpdateSettings(ctx context.Context, req *dto.AdmissionEventSettingsRequest) (*dto.AdmissionEventSettings, error) {
	settings, policies, err := admissionSettingsModels(req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateSettingsAndPolicies(ctx, settings, policies); err != nil {
		return nil, err
	}
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, gatewaypb.AdmissionEventSettingsTopic)
	}
	return s.Settings(ctx)
}

func (s *AdmissionService) List(ctx context.Context, params dto.ListAdmissionEventsParams) (*dto.AdmissionEventListData, error) {
	page := params.Page.Or(1)
	if page < 1 {
		return nil, &ValidationError{Msg: "invalid page: must be greater than zero"}
	}
	pageSize := int(params.PageSize.Or(dto.PageSize20))
	options := dbrepo.AdmissionEventPageOptions{
		PageOptions: dbrepo.PageOptions{
			Limit:  pageSize,
			Offset: (page - 1) * pageSize,
			SortBy: string(params.SortBy.Or(dto.ListAdmissionEventsSortByOccurredAt)),
			Desc:   params.SortOrder.Or(dto.ListAdmissionEventsSortOrderDesc) == dto.ListAdmissionEventsSortOrderDesc,
		},
		ClusterNames: params.Cluster,
		PolicyName:   params.Policy.Or(""),
		Namespaces:   params.Namespace,
		GVKs:         params.Gvk,
		ResourceName: params.ResourceName.Or(""),
		Username:     params.Username.Or(""),
	}
	for _, verdict := range params.Verdict {
		options.Verdicts = append(options.Verdicts, string(verdict))
	}
	if value, ok := params.From.Get(); ok {
		options.From = &value
	}
	if value, ok := params.To.Get(); ok {
		options.To = &value
	}
	rows, total, err := s.repo.ListEventsPage(ctx, options)
	if err != nil {
		return nil, err
	}
	items := make([]dto.AdmissionEventItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, admissionItem(row))
	}
	return &dto.AdmissionEventListData{Total: int(total), Page: page, PageSize: dto.AdmissionEventListDataPageSize(pageSize), Items: items}, nil
}

func (s *AdmissionService) Get(ctx context.Context, resourceID dto.ResourceID) (*dto.AdmissionEventDetail, error) {
	id, err := strconv.ParseInt(string(resourceID), 10, 64)
	if err != nil || id <= 0 {
		return nil, &ValidationError{Msg: "invalid event id"}
	}
	event, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	detail := admissionDetail(*event)
	return &detail, nil
}

func (s *AdmissionService) Trend(ctx context.Context, from, to time.Time) ([]dto.AdmissionEventTrendItem, error) {
	if to.Before(from) {
		return nil, &ValidationError{Msg: "to must be after from"}
	}
	rows, err := s.repo.Trend(ctx, from, to, to.Sub(from) <= 48*time.Hour)
	if err != nil {
		return nil, err
	}
	items := make([]dto.AdmissionEventTrendItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.AdmissionEventTrendItem{Bucket: row.Bucket, Verdict: dto.AdmissionEventTrendItemVerdict(row.Verdict), Count: row.Count})
	}
	return items, nil
}

func admissionItem(event dbrepo.AdmissionEvent) dto.AdmissionEventItem {
	violations := []string{}
	_ = json.Unmarshal(event.Violations, &violations)
	policyNames := []string(event.PolicyNames)
	if len(policyNames) == 0 {
		policyNames, _ = webhook.AdmissionEventReferences(violations)
	}
	return dto.AdmissionEventItem{
		ID: dto.ResourceID(strconv.FormatInt(event.ID, 10)), EventId: event.EventID, ClusterName: event.ClusterName, OccurredAt: event.OccurredAt,
		Verdict: dto.AdmissionEventItemVerdict(event.Verdict), ApiGroup: event.APIGroup, ApiVersion: event.APIVersion, Resource: event.Resource, Operation: event.Operation,
		Namespace: event.Namespace, ResourceName: event.ResourceName, Username: event.Username, PolicyNames: policyNames, Violations: violations,
	}
}

func admissionDetail(event dbrepo.AdmissionEvent) dto.AdmissionEventDetail {
	base := admissionItem(event)
	return dto.AdmissionEventDetail{
		ID: base.ID, EventId: base.EventId, ClusterName: base.ClusterName, OccurredAt: base.OccurredAt,
		Verdict: dto.AdmissionEventDetailVerdict(event.Verdict), ApiGroup: base.ApiGroup, ApiVersion: base.ApiVersion, Resource: base.Resource,
		Operation: base.Operation, Namespace: base.Namespace, ResourceName: base.ResourceName, Username: base.Username, PolicyNames: base.PolicyNames,
		Violations: base.Violations, RequestUid: event.RequestUID, UserGroups: []string(event.UserGroups), PolicyGroupNames: []string(event.PolicyGroupNames),
		MetadataSnapshot: dto.AdmissionEventDetailMetadataSnapshot(rawAdmissionObject(event.MetadataSnapshot)),
		SpecSnapshot:     dto.AdmissionEventDetailSpecSnapshot(rawAdmissionObject(event.SpecSnapshot)), StoragePolicyVersion: event.StoragePolicyVersion,
	}
}

func rawAdmissionObject(raw []byte) map[string]jx.Raw {
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

func admissionSettings(settings *dbrepo.AdmissionEventSettings, policies []dbrepo.AdmissionStoragePolicy) dto.AdmissionEventSettings {
	result := make([]dto.AdmissionStoragePolicy, 0, len(policies))
	for _, policy := range policies {
		verdicts := make([]dto.AdmissionStoragePolicyVerdictsItem, 0, len(policy.Verdicts))
		for _, verdict := range policy.Verdicts {
			verdicts = append(verdicts, dto.AdmissionStoragePolicyVerdictsItem(verdict))
		}
		matchMode := policy.MatchMode
		if matchMode == "" {
			matchMode = dbrepo.AdmissionMatchModeInclude
		}
		result = append(result, dto.AdmissionStoragePolicy{MatchMode: dto.AdmissionStoragePolicyMatchMode(matchMode), Verdicts: verdicts, Clusters: []string(policy.Clusters), Gvks: []string(policy.GVKs), Namespaces: []string(policy.Namespaces)})
	}
	return dto.AdmissionEventSettings{
		DefaultRetentionDays: settings.DefaultRetentionDays, RetryEnabled: settings.RetryEnabled, MaxRetryAttempts: settings.MaxRetryAttempts,
		QueueCapacity: settings.QueueCapacity, IncidentRetentionDays: settings.IncidentRetentionDays, ObservationEnabled: settings.ObservationEnabled,
		StoragePolicyVersion: settings.StoragePolicyVersion, StoragePolicies: result,
	}
}

func admissionSettingsModels(req *dto.AdmissionEventSettingsRequest) (*dbrepo.AdmissionEventSettings, []dbrepo.AdmissionStoragePolicy, error) {
	maxRetryAttempts := req.MaxRetryAttempts.Or(3)
	queueCapacity := req.QueueCapacity.Or(1024)
	incidentRetentionDays := req.IncidentRetentionDays.Or(7)
	if req.DefaultRetentionDays < 1 || req.DefaultRetentionDays > 365 {
		return nil, nil, &ValidationError{Msg: "defaultRetentionDays must be between 1 and 365"}
	}
	if maxRetryAttempts < 1 || maxRetryAttempts > 100 {
		return nil, nil, &ValidationError{Msg: "maxRetryAttempts must be between 1 and 100"}
	}
	if queueCapacity < 1 || queueCapacity > 10000 {
		return nil, nil, &ValidationError{Msg: "queueCapacity must be between 1 and 10000"}
	}
	if incidentRetentionDays < 1 || incidentRetentionDays > 365 {
		return nil, nil, &ValidationError{Msg: "incidentRetentionDays must be between 1 and 365"}
	}
	policies := make([]dbrepo.AdmissionStoragePolicy, 0, len(req.StoragePolicies))
	for index, policy := range req.StoragePolicies {
		matchMode := string(policy.MatchMode)
		if matchMode == "" {
			matchMode = dbrepo.AdmissionMatchModeInclude
		}
		if matchMode != dbrepo.AdmissionMatchModeInclude && matchMode != dbrepo.AdmissionMatchModeExclude {
			return nil, nil, &ValidationError{Msg: "storage policy matchMode must be include or exclude"}
		}
		verdicts := make([]string, 0, len(policy.Verdicts))
		for _, verdict := range policy.Verdicts {
			if verdict != dto.AdmissionStoragePolicyVerdictsItemAudit && verdict != dto.AdmissionStoragePolicyVerdictsItemDeny {
				return nil, nil, &ValidationError{Msg: "storage policy verdict must be audit or deny"}
			}
			verdicts = append(verdicts, string(verdict))
		}
		for _, gvr := range policy.Gvks {
			if !validAdmissionGVR(gvr) {
				return nil, nil, &ValidationError{Msg: "storage policy gvk must be a normalized resource such as pods or apps/deployments"}
			}
		}
		policies = append(policies, dbrepo.AdmissionStoragePolicy{Priority: index + 1, MatchMode: matchMode, Verdicts: verdicts, Clusters: policy.Clusters, GVKs: policy.Gvks, Namespaces: policy.Namespaces})
	}
	return &dbrepo.AdmissionEventSettings{
		ID: 1, DefaultRetentionDays: req.DefaultRetentionDays, RetryEnabled: req.RetryEnabled.Or(false), MaxRetryAttempts: maxRetryAttempts,
		QueueCapacity: queueCapacity, IncidentRetentionDays: incidentRetentionDays, ObservationEnabled: req.ObservationEnabled.Or(false),
	}, policies, nil
}

var admissionResourcePart = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)

func validAdmissionGVR(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Count(value, "/") > 1 {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if !admissionResourcePart.MatchString(part) {
			return false
		}
	}
	return true
}
