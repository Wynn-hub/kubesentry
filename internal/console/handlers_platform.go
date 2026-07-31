package console

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

const (
	platformNoDataAfter = 3 * time.Minute
	platformHealthRange = 5 * time.Minute
)

type platformInstanceResponse struct {
	Instance         string         `json:"instance"`
	Role             string         `json:"role"`
	Health           string         `json:"health"`
	HealthReason     string         `json:"healthReason"`
	UptimeSeconds    int64          `json:"uptimeSeconds"`
	RestartCount     int64          `json:"restartCount"`
	MemoryBytes      int64          `json:"memoryBytes"`
	MemoryLimitBytes int64          `json:"memoryLimitBytes"`
	CPUMillis        int64          `json:"cpuMillis"`
	Goroutines       int64          `json:"goroutines"`
	HeapAllocBytes   int64          `json:"heapAllocBytes"`
	HeapSysBytes     int64          `json:"heapSysBytes"`
	LastObservedAt   time.Time      `json:"lastObservedAt"`
	DBStats          map[string]any `json:"dbStats,omitempty"`
}

type platformComponentResponse struct {
	Component            string                     `json:"component"`
	Health               string                     `json:"health"`
	HasLeadership        bool                       `json:"hasLeadership"`
	Instances            []platformInstanceResponse `json:"instances"`
	LastLeadershipChange *time.Time                 `json:"lastLeadershipChange"`
}

type platformClusterResponse struct {
	Name       string                      `json:"name"`
	Phase      string                      `json:"phase"`
	Health     string                      `json:"health"`
	Components []platformComponentResponse `json:"components"`
}

type platformOverviewResponse struct {
	PlatformHealth string                      `json:"platformHealth"`
	GatewayPlane   []platformComponentResponse `json:"gatewayPlane"`
	MemberPlane    struct {
		Clusters []platformClusterResponse `json:"clusters"`
		Total    int                       `json:"total"`
		Page     int                       `json:"page"`
		PageSize int                       `json:"pageSize"`
	} `json:"memberPlane"`
}

func (h *Handlers) platformOverview(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Platform == nil || h.Repos.Clusters == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	page, pageSize, ok := observationPage(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC()
	samples, err := h.Repos.Platform.LatestSamples(r.Context(), now.Add(-dbrepo.PlatformSampleRetention))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	restartedRows, err := h.Repos.Platform.RestartedInstances(r.Context(), now.Add(-platformHealthRange))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	restarted := map[string]bool{}
	for _, row := range restartedRows {
		restarted[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = true
	}
	cpuRows, err := h.Repos.Platform.LatestCPUUsage(r.Context(), now.Add(-dbrepo.PlatformSampleRetention))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cpuUsage := map[string]int64{}
	for _, row := range cpuRows {
		cpuUsage[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = row.CPUUsageMicros
	}
	events, err := h.Repos.Platform.LatestLeadershipEvents(
		r.Context(), now.Add(-dbrepo.PlatformLeadershipRetention))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	lastChanges := map[string]time.Time{}
	for _, event := range events {
		key := platformComponentKey(event.Component, event.ClusterID)
		if _, exists := lastChanges[key]; !exists {
			lastChanges[key] = event.ChangedAt
		}
	}
	grouped := groupPlatformSamples(samples)
	response := platformOverviewResponse{}
	response.GatewayPlane = make([]platformComponentResponse, 0, 4)
	for _, component := range []platformobs.Component{
		platformobs.ComponentGateway, platformobs.ComponentConsoleAPI,
		platformobs.ComponentConsoleWeb, platformobs.ComponentPostgres,
	} {
		response.GatewayPlane = append(response.GatewayPlane,
			buildPlatformComponent(component, grouped[platformComponentKey(string(component), 0)],
				lastChanges[platformComponentKey(string(component), 0)], restarted, cpuUsage, now))
	}
	response.PlatformHealth = "ok"
	for _, component := range response.GatewayPlane {
		response.PlatformHealth = worsePlatformHealth(response.PlatformHealth, component.Health)
	}

	clusters, err := h.Repos.Clusters.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	filter := strings.TrimSpace(r.URL.Query().Get("health"))
	all := make([]platformClusterResponse, 0, len(clusters))
	for _, cluster := range clusters {
		components := []platformComponentResponse{
			buildPlatformComponent(platformobs.ComponentWebhook,
				grouped[platformComponentKey(string(platformobs.ComponentWebhook), cluster.ID)],
				lastChanges[platformComponentKey(string(platformobs.ComponentWebhook), cluster.ID)], restarted, cpuUsage, now),
			buildPlatformComponent(platformobs.ComponentOperator,
				grouped[platformComponentKey(string(platformobs.ComponentOperator), cluster.ID)],
				lastChanges[platformComponentKey(string(platformobs.ComponentOperator), cluster.ID)], restarted, cpuUsage, now),
		}
		health := "ok"
		for _, component := range components {
			health = worsePlatformHealth(health, component.Health)
		}
		if filter != "" && health != filter {
			continue
		}
		all = append(all, platformClusterResponse{
			Name: cluster.Name, Phase: cluster.Phase, Health: health, Components: components,
		})
	}
	response.MemberPlane.Total = len(all)
	response.MemberPlane.Page = page
	response.MemberPlane.PageSize = pageSize
	start := (page - 1) * pageSize
	if start < len(all) {
		end := start + pageSize
		if end > len(all) {
			end = len(all)
		}
		response.MemberPlane.Clusters = all[start:end]
	} else {
		response.MemberPlane.Clusters = []platformClusterResponse{}
	}
	writeOK(w, response)
}

func groupPlatformSamples(rows []dbrepo.PlatformComponentSample) map[string][]dbrepo.PlatformComponentSample {
	result := map[string][]dbrepo.PlatformComponentSample{}
	for _, row := range rows {
		key := platformComponentKey(row.Component, row.ClusterID)
		result[key] = append(result[key], row)
	}
	return result
}

func platformComponentKey(component string, clusterID int64) string {
	return component + "/" + strconv.FormatInt(clusterID, 10)
}

func platformInstanceKey(component, instance string, clusterID int64) string {
	return platformComponentKey(component, clusterID) + "/" + instance
}

func buildPlatformComponent(
	component platformobs.Component,
	rows []dbrepo.PlatformComponentSample,
	lastChange time.Time,
	restarted map[string]bool,
	cpuUsage map[string]int64,
	now time.Time,
) platformComponentResponse {
	hasLeadership := component == platformobs.ComponentGateway || component == platformobs.ComponentOperator
	result := platformComponentResponse{
		Component: string(component), Health: "unavailable",
		HasLeadership: hasLeadership, Instances: make([]platformInstanceResponse, 0, len(rows)),
	}
	if !lastChange.IsZero() {
		result.LastLeadershipChange = &lastChange
	}
	leaderCount, noDataCount := 0, 0
	for _, row := range rows {
		health := row.Health
		reason := row.HealthReason
		if now.Sub(row.LastObservedAt) > platformNoDataAfter {
			health, reason = "no_data", "sample_stale"
			if component == platformobs.ComponentConsoleWeb {
				reason = "sample_stale_collected_by_console_api"
			}
			noDataCount++
		} else if row.MemoryLimitBytes > 0 &&
			row.MemoryBytes > row.MemoryLimitBytes-row.MemoryLimitBytes/10 {
			health, reason = "degraded", "memory_limit_pressure"
		} else if restarted[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] {
			health, reason = "degraded", "restarted_in_health_window"
		}
		if row.Role == string(platformobs.RoleLeader) && health != "no_data" {
			leaderCount++
		}
		uptime := int64(now.Sub(row.ProcessStartedAt).Seconds())
		if uptime < 0 {
			uptime = 0
		}
		cpuMillis := int64(-1)
		if value, exists := cpuUsage[platformInstanceKey(row.Component, row.Instance, row.ClusterID)]; exists && value >= 0 {
			cpuMillis = value / 1000
		}
		result.Instances = append(result.Instances, platformInstanceResponse{
			Instance: row.Instance, Role: row.Role, Health: health, HealthReason: reason,
			UptimeSeconds: uptime, RestartCount: row.RestartCount,
			MemoryBytes: row.MemoryBytes, MemoryLimitBytes: row.MemoryLimitBytes,
			CPUMillis:      cpuMillis,
			HeapAllocBytes: row.HeapAllocBytes, HeapSysBytes: row.HeapSysBytes,
			LastObservedAt: row.LastObservedAt, DBStats: dbrepo.DecodeDBStats(row),
		})
	}
	switch {
	case len(rows) == 0 || noDataCount == len(rows):
		result.Health = "unavailable"
	case noDataCount > 0:
		result.Health = "degraded"
	default:
		result.Health = "ok"
		for _, instance := range result.Instances {
			if instance.Health == "degraded" {
				result.Health = "degraded"
			}
		}
	}
	if hasLeadership && result.Health != "unavailable" && leaderCount != 1 {
		result.Health = "degraded"
	}
	return result
}

func worsePlatformHealth(current, candidate string) string {
	rank := map[string]int{"ok": 0, "degraded": 1, "unavailable": 2}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
}

func (h *Handlers) platformInstanceTrend(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Platform == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	from, to, ok := platformWindow(w, r, dbrepo.PlatformSampleRetention, 24*time.Hour)
	if !ok {
		return
	}
	clusterID := int64(0)
	if raw := r.URL.Query().Get("cluster"); raw != "" {
		cluster, err := h.Repos.Clusters.GetByName(r.Context(), raw)
		if err != nil {
			writeErr(w, http.StatusNotFound, "cluster not found")
			return
		}
		clusterID = cluster.ID
	}
	step := observationTrendStep(to.Sub(from))
	rows, err := h.Repos.Platform.Trend(
		r.Context(), r.PathValue("component"), r.PathValue("instance"),
		clusterID, from, to, step)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{
		"items": rows, "stepSeconds": int64(step / time.Second), "from": from, "to": to,
	})
}

func (h *Handlers) platformLeadershipEvents(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Platform == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	page, pageSize, ok := observationPage(w, r)
	if !ok {
		return
	}
	from, to, ok := platformWindow(w, r, dbrepo.PlatformLeadershipRetention, 24*time.Hour)
	if !ok {
		return
	}
	var clusterID *int64
	if name := r.URL.Query().Get("cluster"); name != "" {
		cluster, err := h.Repos.Clusters.GetByName(r.Context(), name)
		if err != nil {
			writeErr(w, http.StatusNotFound, "cluster not found")
			return
		}
		clusterID = &cluster.ID
	}
	result, err := h.Repos.Platform.LeadershipEvents(
		r.Context(), r.URL.Query().Get("component"), clusterID, from, to, page, pageSize)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, map[string]any{
		"items": result.Items, "total": result.Total, "page": page, "pageSize": pageSize,
	})
}

type platformSettingsResponse struct {
	AdmissionPipelineEnabled bool `json:"admissionPipelineEnabled"`
}

func (h *Handlers) getPlatformSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Platform == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	settings, err := h.Repos.Platform.GetSettings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, platformSettingsResponse{AdmissionPipelineEnabled: settings.AdmissionPipelineEnabled})
}

func (h *Handlers) updatePlatformSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Platform == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	var request platformSettingsResponse
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Repos.Platform.SetAdmissionPipelineEnabled(r.Context(), request.AdmissionPipelineEnabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.clearObservationCache()
	h.notifyChange(r, gateway.PlatformObservabilitySettingsTopic)
	h.getPlatformSettings(w, r)
}

func platformWindow(
	w http.ResponseWriter,
	r *http.Request,
	retention time.Duration,
	defaultSpan time.Duration,
) (time.Time, time.Time, bool) {
	now := time.Now().UTC()
	to := now
	from := now.Add(-defaultSpan)
	var err error
	if value := r.URL.Query().Get("to"); value != "" {
		to, err = time.Parse(time.RFC3339, value)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "to must be RFC3339")
			return time.Time{}, time.Time{}, false
		}
	}
	if value := r.URL.Query().Get("from"); value != "" {
		from, err = time.Parse(time.RFC3339, value)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "from must be RFC3339")
			return time.Time{}, time.Time{}, false
		}
	}
	if to.After(now) {
		to = now
	}
	if cutoff := now.Add(-retention); from.Before(cutoff) {
		from = cutoff
	}
	from, to = from.UTC(), to.UTC()
	if !from.Before(to) {
		writeErr(w, http.StatusBadRequest, "from must be before to within the retention period")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}
