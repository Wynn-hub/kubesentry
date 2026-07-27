package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type admissionStoragePolicyInput struct {
	Verdicts      []string `json:"verdicts"`
	Clusters      []string `json:"clusters"`
	GVKs          []string `json:"gvks"`
	Namespaces    []string `json:"namespaces"`
	Action        string   `json:"action"`
	RetentionDays *int     `json:"retentionDays,omitempty"`
}
type admissionSettingsRequest struct {
	DefaultRetentionDays int                           `json:"defaultRetentionDays"`
	StoragePolicies      []admissionStoragePolicyInput `json:"storagePolicies"`
}
type admissionSettingsResponse struct {
	DefaultRetentionDays int                           `json:"defaultRetentionDays"`
	StoragePolicyVersion int64                         `json:"storagePolicyVersion"`
	StoragePolicies      []admissionStoragePolicyInput `json:"storagePolicies"`
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
	writeOK(w, admissionSettingsResponse{DefaultRetentionDays: settings.DefaultRetentionDays, StoragePolicyVersion: settings.StoragePolicyVersion, StoragePolicies: storagePolicyInputs(policies)})
}

func (h *Handlers) updateAdmissionEventSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Admission == nil {
		writeErr(w, http.StatusServiceUnavailable, "admission events are unavailable")
		return
	}
	var req admissionSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid request body")
		return
	}
	if req.DefaultRetentionDays < 1 || req.DefaultRetentionDays > 365 {
		writeErr(w, 400, "defaultRetentionDays must be between 1 and 365")
		return
	}
	policies := make([]dbrepo.AdmissionStoragePolicy, 0, len(req.StoragePolicies))
	for i, input := range req.StoragePolicies {
		if input.Action != dbrepo.AdmissionStorageActionRetain && input.Action != dbrepo.AdmissionStorageActionDiscard {
			writeErr(w, 400, "storage policy action must be retain or discard")
			return
		}
		if input.Action == dbrepo.AdmissionStorageActionRetain && (input.RetentionDays == nil || *input.RetentionDays < 1 || *input.RetentionDays > 365) {
			writeErr(w, 400, "retain policy requires retentionDays between 1 and 365")
			return
		}
		if input.Action == dbrepo.AdmissionStorageActionDiscard {
			input.RetentionDays = nil
		}
		policies = append(policies, dbrepo.AdmissionStoragePolicy{Priority: i + 1, Verdicts: pq.StringArray(input.Verdicts), Clusters: pq.StringArray(input.Clusters), GVKs: pq.StringArray(input.GVKs), Namespaces: pq.StringArray(input.Namespaces), Action: input.Action, RetentionDays: input.RetentionDays})
	}
	if err := h.Repos.Admission.UpdateSettingsAndPolicies(r.Context(), &dbrepo.AdmissionEventSettings{ID: 1, DefaultRetentionDays: req.DefaultRetentionDays}, policies); err != nil {
		writeErr(w, 500, "update admission event settings: "+err.Error())
		return
	}
	h.notifyChange(r, "admission-event-settings")
	h.getAdmissionEventSettings(w, r)
}

func storagePolicyInputs(in []dbrepo.AdmissionStoragePolicy) []admissionStoragePolicyInput {
	out := make([]admissionStoragePolicyInput, 0, len(in))
	for _, p := range in {
		out = append(out, admissionStoragePolicyInput{Verdicts: p.Verdicts, Clusters: p.Clusters, GVKs: p.GVKs, Namespaces: p.Namespaces, Action: p.Action, RetentionDays: p.RetentionDays})
	}
	return out
}

type admissionEventItem struct {
	ID           int64           `json:"id"`
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
	return admissionEventItem{ID: e.ID, EventID: e.EventID, ClusterName: e.ClusterName, OccurredAt: e.OccurredAt, Verdict: e.Verdict, APIGroup: e.APIGroup, APIVersion: e.APIVersion, Resource: e.Resource, Operation: e.Operation, Namespace: e.Namespace, ResourceName: e.ResourceName, Username: e.Username, PolicyNames: e.PolicyNames, Violations: json.RawMessage(e.Violations)}
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
	options := dbrepo.AdmissionEventPageOptions{PageOptions: dbrepo.PageOptions{Limit: q.PageSize, Offset: q.offset(), SortBy: q.SortBy, Desc: q.SortOrder == "desc"}, ClusterName: r.URL.Query().Get("cluster"), Verdict: r.URL.Query().Get("verdict"), PolicyName: r.URL.Query().Get("policy"), Namespace: r.URL.Query().Get("namespace"), GVK: r.URL.Query().Get("gvk"), ResourceName: r.URL.Query().Get("resourceName"), Username: r.URL.Query().Get("username")}
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
