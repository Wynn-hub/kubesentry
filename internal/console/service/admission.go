package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

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

func (s *AdmissionService) Settings(ctx context.Context) (*dto.AdmissionSettingsView, error) {
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

func (s *AdmissionService) UpdateSettings(ctx context.Context, req *dto.AdmissionSettingsInput) (*dto.AdmissionSettingsView, error) {
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

func (s *AdmissionService) List(ctx context.Context, params dto.AdmissionEventListOptions) (*dto.AdmissionEventPageView, error) {
	page := params.Page
	if page < 1 {
		return nil, &ValidationError{Msg: "invalid page: must be greater than zero"}
	}
	pageSize := params.PageSize
	options := dbrepo.AdmissionEventPageOptions{
		PageOptions: dbrepo.PageOptions{
			Limit:  pageSize,
			Offset: (page - 1) * pageSize,
			SortBy: params.SortBy,
			Desc:   params.SortOrder == "desc",
		},
		ClusterNames: params.Clusters,
		PolicyName:   params.PolicyName,
		Namespaces:   params.Namespaces,
		GVKs:         params.GVRs,
		ResourceName: params.ResourceName,
		Username:     params.Username,
	}
	options.Verdicts = append(options.Verdicts, params.Verdicts...)
	options.From, options.To = params.From, params.To
	rows, total, err := s.repo.ListEventsPage(ctx, options)
	if err != nil {
		return nil, err
	}
	items := make([]dto.AdmissionEventItemView, 0, len(rows))
	for _, row := range rows {
		items = append(items, admissionItem(row))
	}
	return &dto.AdmissionEventPageView{Total: int(total), Page: page, PageSize: pageSize, Items: items}, nil
}

func (s *AdmissionService) Get(ctx context.Context, resourceID string) (*dto.AdmissionEventDetailView, error) {
	id, err := strconv.ParseInt(resourceID, 10, 64)
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

func (s *AdmissionService) Trend(ctx context.Context, from, to time.Time) ([]dto.AdmissionTrendItemView, error) {
	if to.Before(from) {
		return nil, &ValidationError{Msg: "to must be after from"}
	}
	rows, err := s.repo.Trend(ctx, from, to, to.Sub(from) <= 48*time.Hour)
	if err != nil {
		return nil, err
	}
	items := make([]dto.AdmissionTrendItemView, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.AdmissionTrendItemView{Bucket: row.Bucket, Verdict: row.Verdict, Count: row.Count})
	}
	return items, nil
}

func admissionItem(event dbrepo.AdmissionEvent) dto.AdmissionEventItemView {
	violations := []string{}
	_ = json.Unmarshal(event.Violations, &violations)
	policyNames := []string(event.PolicyNames)
	if len(policyNames) == 0 {
		policyNames, _ = webhook.AdmissionEventReferences(violations)
	}
	return dto.AdmissionEventItemView{
		ID: strconv.FormatInt(event.ID, 10), EventID: event.EventID, ClusterName: event.ClusterName, OccurredAt: event.OccurredAt,
		Verdict: event.Verdict, APIGroup: event.APIGroup, APIVersion: event.APIVersion, Resource: event.Resource, Operation: event.Operation,
		Namespace: event.Namespace, ResourceName: event.ResourceName, Username: event.Username, PolicyNames: policyNames, Violations: violations,
	}
}

func admissionDetail(event dbrepo.AdmissionEvent) dto.AdmissionEventDetailView {
	base := admissionItem(event)
	return dto.AdmissionEventDetailView{
		AdmissionEventItemView: base, RequestUID: event.RequestUID, UserGroups: []string(event.UserGroups), PolicyGroupNames: []string(event.PolicyGroupNames),
		MetadataSnapshot: rawAdmissionObject(event.MetadataSnapshot),
		SpecSnapshot:     rawAdmissionObject(event.SpecSnapshot), StoragePolicyVersion: event.StoragePolicyVersion,
	}
}

func rawAdmissionObject(raw []byte) map[string]json.RawMessage {
	value := map[string]json.RawMessage{}
	if len(raw) == 0 || string(raw) == "null" {
		return value
	}
	var decoded map[string]json.RawMessage
	if json.Unmarshal(raw, &decoded) != nil {
		return value
	}
	for key, item := range decoded {
		value[key] = item
	}
	return value
}

func admissionSettings(settings *dbrepo.AdmissionEventSettings, policies []dbrepo.AdmissionStoragePolicy) dto.AdmissionSettingsView {
	result := make([]dto.AdmissionStoragePolicyView, 0, len(policies))
	for _, policy := range policies {
		verdicts := append([]string(nil), policy.Verdicts...)
		matchMode := policy.MatchMode
		if matchMode == "" {
			matchMode = dbrepo.AdmissionMatchModeInclude
		}
		result = append(result, dto.AdmissionStoragePolicyView{MatchMode: matchMode, Verdicts: verdicts, Clusters: []string(policy.Clusters), GVRs: []string(policy.GVKs), Namespaces: []string(policy.Namespaces)})
	}
	return dto.AdmissionSettingsView{
		DefaultRetentionDays: settings.DefaultRetentionDays, RetryEnabled: settings.RetryEnabled, MaxRetryAttempts: settings.MaxRetryAttempts,
		QueueCapacity: settings.QueueCapacity, IncidentRetentionDays: settings.IncidentRetentionDays, ObservationEnabled: settings.ObservationEnabled,
		StoragePolicyVersion: settings.StoragePolicyVersion, StoragePolicies: result,
	}
}

func admissionSettingsModels(req *dto.AdmissionSettingsInput) (*dbrepo.AdmissionEventSettings, []dbrepo.AdmissionStoragePolicy, error) {
	maxRetryAttempts := intOr(req.MaxRetryAttempts, 3)
	queueCapacity := intOr(req.QueueCapacity, 1024)
	incidentRetentionDays := intOr(req.IncidentRetentionDays, 7)
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
			if verdict != "audit" && verdict != "deny" {
				return nil, nil, &ValidationError{Msg: "storage policy verdict must be audit or deny"}
			}
			verdicts = append(verdicts, string(verdict))
		}
		for _, gvr := range policy.GVRs {
			if !validAdmissionGVR(gvr) {
				return nil, nil, &ValidationError{Msg: "storage policy gvk must be a normalized resource such as pods or apps/deployments"}
			}
		}
		policies = append(policies, dbrepo.AdmissionStoragePolicy{Priority: index + 1, MatchMode: matchMode, Verdicts: verdicts, Clusters: policy.Clusters, GVKs: policy.GVRs, Namespaces: policy.Namespaces})
	}
	return &dbrepo.AdmissionEventSettings{
		ID: 1, DefaultRetentionDays: req.DefaultRetentionDays, RetryEnabled: boolOr(req.RetryEnabled, false), MaxRetryAttempts: maxRetryAttempts,
		QueueCapacity: queueCapacity, IncidentRetentionDays: incidentRetentionDays, ObservationEnabled: boolOr(req.ObservationEnabled, false),
	}, policies, nil
}

func intOr(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
func boolOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
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
