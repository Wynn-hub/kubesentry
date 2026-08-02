package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
	HealthReason         string                     `json:"healthReason"`
	HasLeadership        bool                       `json:"hasLeadership"`
	NoLeader             bool                       `json:"noLeader"`
	SingleReplica        bool                       `json:"singleReplica"`
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
	Enabled        bool                        `json:"enabled"`
	PlatformHealth string                      `json:"platformHealth"`
	GatewayPlane   []platformComponentResponse `json:"gatewayPlane"`
	MemberPlane    struct {
		Clusters []platformClusterResponse `json:"clusters"`
		Total    int                       `json:"total"`
		Page     int                       `json:"page"`
		PageSize int                       `json:"pageSize"`
	} `json:"memberPlane"`
}

// gatewayPlaneOrder follows the data path: samples enter at the gateway, land in
// postgres, and are served by console-api to console-web.
var gatewayPlaneOrder = []platformobs.Component{
	platformobs.ComponentGateway,
	platformobs.ComponentPostgres,
	platformobs.ComponentConsoleAPI,
	platformobs.ComponentConsoleWeb,
}

// memberPlaneOrder puts the management plane before the data plane, matching how
// the architecture is described everywhere else.
var memberPlaneOrder = []platformobs.Component{
	platformobs.ComponentOperator,
	platformobs.ComponentWebhook,
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
	settings, err := h.Repos.Platform.GetSettings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !settings.Enabled {
		// Report the switch state rather than an empty platform: the page needs
		// to distinguish "off" from "on but nothing reporting", which look
		// identical in the data.
		response := platformOverviewResponse{Enabled: false, PlatformHealth: "disabled"}
		response.GatewayPlane = []platformComponentResponse{}
		response.MemberPlane.Clusters = []platformClusterResponse{}
		response.MemberPlane.Page, response.MemberPlane.PageSize = page, pageSize
		writeOK(w, response)
		return
	}
	now := time.Now().UTC()
	retention := time.Duration(settings.RetentionDays) * 24 * time.Hour
	if retention <= 0 || retention > dbrepo.PlatformSampleRetentionMax {
		retention = dbrepo.PlatformSampleRetentionMax
	}
	samples, err := h.Repos.Platform.LatestSamples(r.Context(), now.Add(-retention))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	restartedRows, err := h.Repos.Platform.RestartedInstances(r.Context(), now.Add(-platformHealthRange))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	signals := platformHealthSignals{
		restarted: map[string]bool{},
		pressured: map[string]bool{},
		cpuUsage:  map[string]int64{},
	}
	for _, row := range restartedRows {
		signals.restarted[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = true
	}
	pressuredRows, err := h.Repos.Platform.MemoryPressuredInstances(r.Context(), now.Add(-platformHealthRange))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, row := range pressuredRows {
		signals.pressured[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = true
	}
	cpuRows, err := h.Repos.Platform.LatestCPUUsage(r.Context(), now.Add(-retention))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, row := range cpuRows {
		signals.cpuUsage[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = row.CPUUsageMicros
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
	response := platformOverviewResponse{Enabled: true}
	response.GatewayPlane = make([]platformComponentResponse, 0, len(gatewayPlaneOrder))
	for _, component := range gatewayPlaneOrder {
		response.GatewayPlane = append(response.GatewayPlane,
			buildPlatformComponent(component, grouped[platformComponentKey(string(component), 0)],
				lastChanges[platformComponentKey(string(component), 0)], signals, now))
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
		components := make([]platformComponentResponse, 0, len(memberPlaneOrder))
		for _, component := range memberPlaneOrder {
			components = append(components, buildPlatformComponent(component,
				grouped[platformComponentKey(string(component), cluster.ID)],
				lastChanges[platformComponentKey(string(component), cluster.ID)], signals, now))
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

// platformHealthSignals carries the window-derived facts that a single sample row
// cannot answer on its own, because both are changes in a cumulative counter.
type platformHealthSignals struct {
	restarted map[string]bool
	pressured map[string]bool
	cpuUsage  map[string]int64
}

func buildPlatformComponent(
	component platformobs.Component,
	rows []dbrepo.PlatformComponentSample,
	lastChange time.Time,
	signals platformHealthSignals,
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
		key := platformInstanceKey(row.Component, row.Instance, row.ClusterID)
		health := row.Health
		reason := row.HealthReason
		switch {
		case now.Sub(row.LastObservedAt) > platformNoDataAfter:
			health, reason = "no_data", "sample_stale"
			if component == platformobs.ComponentConsoleWeb {
				reason = "sample_stale_collected_by_console_api"
			}
			noDataCount++
		// Deliberately NOT "memory usage is close to the limit". A Go process
		// grows its RSS toward the cgroup ceiling before GC reclaims, so a high
		// usage ratio is ordinary operation — flagging it marked every healthy
		// console-api degraded. Only the kernel's own verdict counts: an
		// allocation that had to reclaim at the limit, or an OOM kill.
		case signals.pressured[key]:
			health, reason = "degraded", "memory_pressure_events"
		case signals.restarted[key]:
			health, reason = "degraded", "restarted_in_health_window"
		}
		// A degraded instance with no reason gives the operator nothing to do.
		// The producers always set one, but a direct DB edit or an older row
		// could leave it blank.
		if health == "degraded" && reason == "" {
			reason = "unspecified"
		}
		if row.Role == string(platformobs.RoleLeader) && health != "no_data" {
			leaderCount++
		}
		uptime := int64(now.Sub(row.ProcessStartedAt).Seconds())
		if uptime < 0 {
			uptime = 0
		}
		cpuMillis := int64(-1)
		if value, exists := signals.cpuUsage[key]; exists && value >= 0 {
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
	// Leader first, then standby, then by name. Sample rows arrive in map-iteration
	// order, so without this the primary and secondary swap places between polls.
	sort.SliceStable(result.Instances, func(i, j int) bool {
		left, right := result.Instances[i], result.Instances[j]
		if rank := platformRoleRank(left.Role) - platformRoleRank(right.Role); rank != 0 {
			return rank < 0
		}
		return left.Instance < right.Instance
	})
	switch {
	// "Never reported" and "reported, then stopped" look identical in the data
	// — both have no fresh sample — but only the second is a fault. Right after
	// the switch is turned on every component is in the first state for up to a
	// sampling interval, and calling that "unavailable" told the operator to
	// investigate a healthy platform that simply had not reported yet.
	case len(rows) == 0:
		result.Health = "pending"
		result.HealthReason = "component_pending"
	case noDataCount == len(rows):
		result.Health = "unavailable"
		result.HealthReason = "component_no_samples"
	case noDataCount > 0:
		result.Health = "degraded"
		result.HealthReason = "component_partial_no_data"
	default:
		result.Health = "ok"
		for _, instance := range result.Instances {
			if instance.Health == "degraded" {
				result.Health = "degraded"
				// Carry the first instance reason up so the component card can
				// explain itself without the operator opening every instance.
				if result.HealthReason == "" {
					result.HealthReason = instance.HealthReason
				}
			}
		}
	}
	// Skip while pending: a component with no samples has no roles to judge, so
	// asking "who is the leader?" would report a fault before the first report.
	if hasLeadership && result.Health != "unavailable" && result.Health != "pending" && leaderCount != 1 {
		result.Health = "degraded"
		result.NoLeader = true
		result.HealthReason = "no_leader"
	}
	// Never leave a degraded component without a stated reason: an unexplained
	// warning colour is worse than no warning, because there is nothing to act on.
	if result.Health == "degraded" && result.HealthReason == "" {
		result.HealthReason = "unspecified"
	}
	// A single instance holding the leader role is a valid single-replica
	// install, not a missing secondary. Say which it is rather than leaving the
	// standby slot silently empty, which reads as "the secondary disappeared".
	result.SingleReplica = hasLeadership && len(result.Instances) == 1 && !result.NoLeader
	return result
}

func platformRoleRank(role string) int {
	switch role {
	case string(platformobs.RoleLeader):
		return 0
	case string(platformobs.RoleStandby):
		return 1
	default:
		return 2
	}
}

// worsePlatformHealth folds component health into the platform banner. "pending"
// ranks with "ok" on purpose: a component that has not reported its first sample
// yet is not evidence of a problem, and ranking it as a fault made every
// freshly-enabled platform raise an alarm about itself.
func worsePlatformHealth(current, candidate string) string {
	rank := map[string]int{"ok": 0, "pending": 0, "degraded": 1, "unavailable": 2}
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
	from, to, ok := platformWindow(w, r, dbrepo.PlatformSampleRetentionMax, 24*time.Hour)
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
	Enabled          bool `json:"enabled"`
	RetentionDays    int  `json:"retentionDays"`
	MaxRetentionDays int  `json:"maxRetentionDays"`
}

func platformSettingsOf(settings *dbrepo.PlatformObservabilitySettings) platformSettingsResponse {
	return platformSettingsResponse{
		Enabled:          settings.Enabled,
		RetentionDays:    settings.RetentionDays,
		MaxRetentionDays: int(dbrepo.PlatformSampleRetentionMax / (24 * time.Hour)),
	}
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
	writeOK(w, platformSettingsOf(settings))
}

type platformSettingsRequest struct {
	Enabled       bool `json:"enabled"`
	RetentionDays int  `json:"retentionDays"`
}

func (h *Handlers) updatePlatformSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Platform == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	var request platformSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	maximum := int(dbrepo.PlatformSampleRetentionMax / (24 * time.Hour))
	if request.RetentionDays < 1 || request.RetentionDays > maximum {
		writeErr(w, http.StatusBadRequest,
			fmt.Sprintf("retentionDays must be between 1 and %d", maximum))
		return
	}
	if err := h.Repos.Platform.UpdateSettings(r.Context(), request.Enabled, request.RetentionDays); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
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
