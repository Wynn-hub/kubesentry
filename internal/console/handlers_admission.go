package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

// admissionStoragePolicyInput is a conditions-only storage rule. Matching any
// rule stores the event; an empty rule set stores everything. Retention lives in
// the global DefaultRetentionDays, not per rule.
type admissionStoragePolicyInput struct {
	Verdicts   []string `json:"verdicts"`
	Clusters   []string `json:"clusters"`
	GVKs       []string `json:"gvks"`
	Namespaces []string `json:"namespaces"`
	MatchMode  string   `json:"matchMode"`
}
type admissionSettingsRequest struct {
	DefaultRetentionDays  int                           `json:"defaultRetentionDays"`
	RetryEnabled          bool                          `json:"retryEnabled"`
	MaxRetryAttempts      int                           `json:"maxRetryAttempts"`
	QueueCapacity         int                           `json:"queueCapacity"`
	IncidentRetentionDays int                           `json:"incidentRetentionDays"`
	ObservationEnabled    bool                          `json:"observationEnabled"`
	StoragePolicies       []admissionStoragePolicyInput `json:"storagePolicies"`
}
type admissionSettingsResponse struct {
	DefaultRetentionDays  int                           `json:"defaultRetentionDays"`
	RetryEnabled          bool                          `json:"retryEnabled"`
	MaxRetryAttempts      int                           `json:"maxRetryAttempts"`
	QueueCapacity         int                           `json:"queueCapacity"`
	IncidentRetentionDays int                           `json:"incidentRetentionDays"`
	ObservationEnabled    bool                          `json:"observationEnabled"`
	StoragePolicyVersion  int64                         `json:"storagePolicyVersion"`
	StoragePolicies       []admissionStoragePolicyInput `json:"storagePolicies"`
}

func (h *Handlers) getAdmissionEventSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Admission == nil {
		writeErr(w, http.StatusServiceUnavailable, "admission events are unavailable")
		return
	}
	settings, err := h.Repos.Admission.GetSettings(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	policies, err := h.Repos.Admission.ListPolicies(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeOK(w, admissionSettingsResponse{
		DefaultRetentionDays:  settings.DefaultRetentionDays,
		RetryEnabled:          settings.RetryEnabled,
		MaxRetryAttempts:      settings.MaxRetryAttempts,
		QueueCapacity:         settings.QueueCapacity,
		IncidentRetentionDays: settings.IncidentRetentionDays,
		ObservationEnabled:    settings.ObservationEnabled,
		StoragePolicyVersion:  settings.StoragePolicyVersion,
		StoragePolicies:       storagePolicyInputs(policies),
	})
}

func (h *Handlers) updateAdmissionEventSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Admission == nil {
		writeErr(w, http.StatusServiceUnavailable, "admission events are unavailable")
		return
	}
	req := admissionSettingsRequest{
		MaxRetryAttempts:      3,
		QueueCapacity:         1024,
		IncidentRetentionDays: 7,
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid request body")
		return
	}
	if req.DefaultRetentionDays < 1 || req.DefaultRetentionDays > 365 {
		writeErr(w, 400, "defaultRetentionDays must be between 1 and 365")
		return
	}
	if req.MaxRetryAttempts < 1 || req.MaxRetryAttempts > 100 {
		writeErr(w, 400, "maxRetryAttempts must be between 1 and 100")
		return
	}
	if req.QueueCapacity < 1 || req.QueueCapacity > 10000 {
		writeErr(w, 400, "queueCapacity must be between 1 and 10000")
		return
	}
	if req.IncidentRetentionDays < 1 || req.IncidentRetentionDays > 365 {
		writeErr(w, 400, "incidentRetentionDays must be between 1 and 365")
		return
	}
	policies := make([]dbrepo.AdmissionStoragePolicy, 0, len(req.StoragePolicies))
	for i, input := range req.StoragePolicies {
		if input.MatchMode == "" {
			input.MatchMode = dbrepo.AdmissionMatchModeInclude
		}
		if input.MatchMode != dbrepo.AdmissionMatchModeInclude && input.MatchMode != dbrepo.AdmissionMatchModeExclude {
			writeErr(w, 400, "storage policy matchMode must be include or exclude")
			return
		}
		for _, verdict := range input.Verdicts {
			if verdict != "audit" && verdict != "deny" {
				writeErr(w, 400, "storage policy verdict must be audit or deny")
				return
			}
		}
		for _, gvr := range input.GVKs {
			if !validAdmissionGVR(gvr) {
				writeErr(w, 400, "storage policy gvk must be a normalized resource such as pods or apps/deployments")
				return
			}
		}
		policies = append(policies, dbrepo.AdmissionStoragePolicy{Priority: i + 1, MatchMode: input.MatchMode, Verdicts: pq.StringArray(input.Verdicts), Clusters: pq.StringArray(input.Clusters), GVKs: pq.StringArray(input.GVKs), Namespaces: pq.StringArray(input.Namespaces)})
	}
	if err := h.Repos.Admission.UpdateSettingsAndPolicies(r.Context(), &dbrepo.AdmissionEventSettings{
		ID:                    1,
		DefaultRetentionDays:  req.DefaultRetentionDays,
		RetryEnabled:          req.RetryEnabled,
		MaxRetryAttempts:      req.MaxRetryAttempts,
		QueueCapacity:         req.QueueCapacity,
		IncidentRetentionDays: req.IncidentRetentionDays,
		ObservationEnabled:    req.ObservationEnabled,
	}, policies); err != nil {
		writeErr(w, 500, "update admission event settings: "+err.Error())
		return
	}
	// Uses the gateway's own topic constant: this notification is what makes
	// the gateway run a full cleanup pass, so a narrowed rule set drops
	// already-stored events that no longer match instead of waiting out the
	// daily ticker. A drifting literal here would silently disable that.
	// The observation switch lives in these settings, and the overview responses
	// are cached for a few seconds. Without dropping the cache, toggling the
	// switch leaves the page showing the previous state for up to one TTL.
	h.notifyChange(r, gateway.AdmissionEventSettingsTopic)
	h.getAdmissionEventSettings(w, r)
}

func storagePolicyInputs(in []dbrepo.AdmissionStoragePolicy) []admissionStoragePolicyInput {
	out := make([]admissionStoragePolicyInput, 0, len(in))
	for _, p := range in {
		matchMode := p.MatchMode
		if matchMode == "" {
			matchMode = dbrepo.AdmissionMatchModeInclude
		}
		out = append(out, admissionStoragePolicyInput{MatchMode: matchMode, Verdicts: p.Verdicts, Clusters: p.Clusters, GVKs: p.GVKs, Namespaces: p.Namespaces})
	}
	return out
}

var admissionResourcePart = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]*[a-z0-9])?$`)

func validAdmissionGVR(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Count(value, "/") > 1 {
		return false
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if !admissionResourcePart.MatchString(part) {
			return false
		}
	}
	return true
}

type admissionEventItem struct {
	// IDs are Snowflake int64 values and exceed JavaScript's safe integer range.
	// Keep them as strings at the HTTP boundary so a detail request targets the
	// exact database row selected in the list.
	ID           string          `json:"id"`
	EventID      string          `json:"eventId"`
	ClusterName  string          `json:"clusterName"`
	OccurredAt   time.Time       `json:"occurredAt"`
	Verdict      string          `json:"verdict"`
	APIGroup     string          `json:"apiGroup"`
	APIVersion   string          `json:"apiVersion"`
	Resource     string          `json:"resource"`
	Operation    string          `json:"operation"`
	Namespace    string          `json:"namespace"`
	ResourceName string          `json:"resourceName"`
	Username     string          `json:"username"`
	PolicyNames  []string        `json:"policyNames"`
	Violations   json.RawMessage `json:"violations"`
}

type admissionEventDetail struct {
	admissionEventItem
	RequestUID           string          `json:"requestUid"`
	UserGroups           []string        `json:"userGroups"`
	PolicyGroupNames     []string        `json:"policyGroupNames"`
	MetadataSnapshot     json.RawMessage `json:"metadataSnapshot"`
	SpecSnapshot         json.RawMessage `json:"specSnapshot"`
	StoragePolicyVersion string          `json:"storagePolicyVersion"`
}

func admissionEventItemOf(e dbrepo.AdmissionEvent) admissionEventItem {
	policyNames := []string(e.PolicyNames)
	if len(policyNames) == 0 {
		var violations []string
		if err := json.Unmarshal(e.Violations, &violations); err == nil {
			policyNames, _ = webhook.AdmissionEventReferences(violations)
		}
	}
	return admissionEventItem{ID: strconv.FormatInt(e.ID, 10), EventID: e.EventID, ClusterName: e.ClusterName, OccurredAt: e.OccurredAt, Verdict: e.Verdict, APIGroup: e.APIGroup, APIVersion: e.APIVersion, Resource: e.Resource, Operation: e.Operation, Namespace: e.Namespace, ResourceName: e.ResourceName, Username: e.Username, PolicyNames: policyNames, Violations: json.RawMessage(e.Violations)}
}

func admissionEventDetailOf(e dbrepo.AdmissionEvent) admissionEventDetail {
	return admissionEventDetail{admissionEventItem: admissionEventItemOf(e), RequestUID: e.RequestUID, UserGroups: e.UserGroups, PolicyGroupNames: e.PolicyGroupNames, MetadataSnapshot: json.RawMessage(e.MetadataSnapshot), SpecSnapshot: json.RawMessage(e.SpecSnapshot), StoragePolicyVersion: e.StoragePolicyVersion}
}

func (h *Handlers) listAdmissionEvents(w http.ResponseWriter, r *http.Request) {
	q, err := parsePageQuery(r.URL.Query(), pageSpec{DefaultSortBy: "occurredAt", DefaultSortOrder: "desc", AllowedSort: stringSet("occurredAt", "createdAt", "verdict")})
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	options := dbrepo.AdmissionEventPageOptions{PageOptions: dbrepo.PageOptions{Limit: q.PageSize, Offset: q.offset(), SortBy: q.SortBy, Desc: q.SortOrder == "desc"}, ClusterNames: splitCSVParam(r.URL.Query().Get("cluster")), Verdicts: splitCSVParam(r.URL.Query().Get("verdict")), PolicyName: r.URL.Query().Get("policy"), Namespaces: splitCSVParam(r.URL.Query().Get("namespace")), GVKs: splitCSVParam(r.URL.Query().Get("gvk")), ResourceName: r.URL.Query().Get("resourceName"), Username: r.URL.Query().Get("username")}
	if options.From, err = parseOptionalTime(r.URL.Query().Get("from")); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if options.To, err = parseOptionalTime(r.URL.Query().Get("to")); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	items, total, err := h.Repos.Admission.ListEventsPage(r.Context(), options)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]admissionEventItem, 0, len(items))
	for _, e := range items {
		out = append(out, admissionEventItemOf(e))
	}
	writeOK(w, pageResult[admissionEventItem]{Items: out, Total: int(total), Page: q.Page, PageSize: q.PageSize})
}

func (h *Handlers) getAdmissionEvent(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid event id")
		return
	}
	e, err := h.Repos.Admission.GetEvent(r.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			writeErr(w, 404, "admission event not found")
		} else {
			writeErr(w, 500, err.Error())
		}
		return
	}
	writeOK(w, admissionEventDetailOf(*e))
}

func (h *Handlers) admissionEventTrend(w http.ResponseWriter, r *http.Request) {
	from, err := parseOptionalTime(r.URL.Query().Get("from"))
	if err != nil || from == nil {
		writeErr(w, 400, "from is required and must be RFC3339")
		return
	}
	to, err := parseOptionalTime(r.URL.Query().Get("to"))
	if err != nil || to == nil {
		writeErr(w, 400, "to is required and must be RFC3339")
		return
	}
	rows, err := h.Repos.Admission.Trend(r.Context(), *from, *to, to.Sub(*from) <= 48*time.Hour)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeOK(w, rows)
}

func parseOptionalTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, fmt.Errorf("invalid RFC3339 time %q", raw)
	}
	return &value, nil
}

// splitCSVParam turns a comma-separated query value into a trimmed,
// empty-free slice. "audit,deny" -> ["audit","deny"]; "" -> nil.
func splitCSVParam(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
