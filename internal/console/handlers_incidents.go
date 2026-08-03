package console

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type linkIncidentItem struct {
	ID                 string    `json:"id"`
	ClusterName        string    `json:"clusterName"`
	ReporterInstanceID string    `json:"reporterInstanceId"`
	Component          string    `json:"component"`
	Reason             string    `json:"reason"`
	StartedAt          time.Time `json:"startedAt"`
	LastSeenAt         time.Time `json:"lastSeenAt"`
	AffectedCount      int64     `json:"affectedCount"`
	Status             string    `json:"status"`
}

type linkIncidentSample struct {
	EventID         string `json:"eventId"`
	EventDatabaseID string `json:"eventDatabaseId,omitempty"`
	Expired         bool   `json:"expired"`
}

type linkIncidentDetail struct {
	linkIncidentItem
	Detail  string               `json:"detail"`
	Samples []linkIncidentSample `json:"samples"`
}

func (h *Handlers) listAdmissionLinkIncidents(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Incidents == nil {
		writeErr(w, http.StatusServiceUnavailable, "admission link incidents are unavailable")
		return
	}
	page, pageSize, ok := observationPage(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	components := csvValues(r.URL.Query().Get("component"))
	for _, component := range components {
		if !admissionobs.Component(component).Valid() {
			writeErr(w, 400, "invalid component")
			return
		}
	}
	reasons := csvValues(r.URL.Query().Get("reason"))
	for _, reason := range reasons {
		if !admissionobs.ValidReason(reason) {
			writeErr(w, 400, "invalid reason")
			return
		}
	}
	statuses := csvValues(r.URL.Query().Get("status"))
	for _, status := range statuses {
		if status != "active" && status != "interrupted" && status != "closed" {
			writeErr(w, 400, "invalid status")
			return
		}
	}
	from, ok := optionalRFC3339(w, r.URL.Query().Get("from"), "from")
	if !ok {
		return
	}
	to, ok := optionalRFC3339(w, r.URL.Query().Get("to"), "to")
	if !ok {
		return
	}
	if from != nil && to != nil && from.After(*to) {
		writeErr(w, 400, "from must not be after to")
		return
	}
	items, total, err := h.Repos.Incidents.ListPage(r.Context(), dbrepo.IncidentPageOptions{
		PageOptions:  dbrepo.PageOptions{Limit: pageSize, Offset: (page - 1) * pageSize, SortBy: "lastSeenAt", Desc: true},
		ClusterNames: csvValues(r.URL.Query().Get("cluster")), Components: components,
		Reasons: reasons, Statuses: statuses, From: from, To: to, Now: now,
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	result := make([]linkIncidentItem, 0, len(items))
	for _, item := range items {
		result = append(result, incidentItem(item, now))
	}
	writeOK(w, map[string]any{"items": result, "total": total, "page": page, "pageSize": pageSize})
}

func (h *Handlers) getAdmissionLinkIncident(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Incidents == nil {
		writeErr(w, http.StatusServiceUnavailable, "admission link incidents are unavailable")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, 400, "invalid incident id")
		return
	}
	incident, err := h.Repos.Incidents.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErr(w, 404, "incident not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	resolved := map[string]int64{}
	if h.Repos.Admission != nil {
		resolved, err = h.Repos.Admission.EventDatabaseIDs(r.Context(), incident.SampleEventIDs)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	samples := make([]linkIncidentSample, 0, len(incident.SampleEventIDs))
	for _, eventID := range incident.SampleEventIDs {
		databaseID, found := resolved[eventID]
		sample := linkIncidentSample{EventID: eventID, Expired: !found}
		if found {
			sample.EventDatabaseID = strconv.FormatInt(databaseID, 10)
		}
		samples = append(samples, sample)
	}
	writeOK(w, linkIncidentDetail{linkIncidentItem: incidentItem(*incident, time.Now().UTC()), Detail: incident.Detail, Samples: samples})
}

func incidentItem(value dbrepo.AdmissionLinkIncident, now time.Time) linkIncidentItem {
	return linkIncidentItem{
		ID: strconv.FormatInt(value.ID, 10), ClusterName: value.ClusterName,
		ReporterInstanceID: value.ReporterInstanceID, Component: value.Component, Reason: value.Reason,
		StartedAt: value.StartedAt, LastSeenAt: value.LastSeenAt, AffectedCount: value.AffectedCount,
		Status: incidentStatus(value, now),
	}
}

func incidentStatus(value dbrepo.AdmissionLinkIncident, now time.Time) string {
	if value.Closed {
		return "closed"
	}
	if value.LastReceivedAt.Before(now.Add(-admissionobs.ActiveFreshness)) {
		return "interrupted"
	}
	return "active"
}

func csvValues(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func optionalRFC3339(w http.ResponseWriter, value, name string) (*time.Time, bool) {
	if value == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		writeErr(w, 400, name+" must be RFC3339")
		return nil, false
	}
	parsed = parsed.UTC()
	return &parsed, true
}

func observationPage(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page, pageSize := 1, 20
	var err error
	if value := r.URL.Query().Get("page"); value != "" {
		page, err = strconv.Atoi(value)
	}
	if err == nil {
		if value := r.URL.Query().Get("pageSize"); value != "" {
			pageSize, err = strconv.Atoi(value)
		}
	}
	if err != nil || page < 1 || pageSize < 1 || pageSize > 100 {
		writeErr(w, http.StatusBadRequest, "page must be positive and pageSize must be between 1 and 100")
		return 0, 0, false
	}
	return page, pageSize, true
}

func observationTrendStep(span time.Duration) time.Duration {
	switch {
	case span <= time.Hour:
		return time.Minute
	case span <= 6*time.Hour:
		return 2 * time.Minute
	case span <= 24*time.Hour:
		return 5 * time.Minute
	case span <= 3*24*time.Hour:
		return 15 * time.Minute
	case span <= 7*24*time.Hour:
		return time.Hour
	case span <= 30*24*time.Hour:
		return 6 * time.Hour
	case span <= 90*24*time.Hour:
		return 24 * time.Hour
	default:
		return 3 * 24 * time.Hour
	}
}
