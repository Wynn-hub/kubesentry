package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	platformobs "github.com/Wynn-hub/kubesentry/internal/platform/observability"
)

const (
	platformNoDataAfter = 3 * time.Minute
	platformHealthRange = 5 * time.Minute
)

var gatewayPlaneOrder = []platformobs.Component{
	platformobs.ComponentGateway,
	platformobs.ComponentPostgres,
	platformobs.ComponentConsoleAPI,
	platformobs.ComponentConsoleWeb,
}

var memberPlaneOrder = []platformobs.Component{
	platformobs.ComponentOperator,
	platformobs.ComponentWebhook,
}

type PlatformService struct {
	platform repository.PlatformRepository
	clusters repository.PlatformClusterRepository
	notifier Notifier
	now      func() time.Time
}

func NewPlatformService(platform repository.PlatformRepository, clusters repository.PlatformClusterRepository, notifier Notifier) *PlatformService {
	return &PlatformService{
		platform: platform,
		clusters: clusters,
		notifier: notifier,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s *PlatformService) Overview(ctx context.Context, p dto.PlatformOverviewOptions) (dto.PlatformOverviewView, error) {
	if s.platform == nil || s.clusters == nil {
		return dto.PlatformOverviewView{}, errors.New("platform observability is unavailable")
	}
	page, pageSize := p.Page, p.PageSize
	settings, err := s.platform.GetSettings(ctx)
	if err != nil {
		return dto.PlatformOverviewView{}, err
	}
	if !settings.Enabled {
		return dto.PlatformOverviewView{
			PlatformHealth: "disabled",
			GatewayPlane:   []dto.PlatformComponentView{},
			MemberPlane: dto.PlatformMemberPlaneView{
				Clusters: []dto.PlatformClusterView{}, Page: page, PageSize: pageSize,
			},
		}, nil
	}

	now := s.now().UTC()
	retention := time.Duration(settings.RetentionDays) * 24 * time.Hour
	if retention <= 0 || retention > dbrepo.PlatformSampleRetentionMax {
		retention = dbrepo.PlatformSampleRetentionMax
	}
	samples, err := s.platform.LatestSamples(ctx, now.Add(-retention))
	if err != nil {
		return dto.PlatformOverviewView{}, err
	}
	restartedRows, err := s.platform.RestartedInstances(ctx, now.Add(-platformHealthRange))
	if err != nil {
		return dto.PlatformOverviewView{}, err
	}
	signals := platformHealthSignals{
		restarted: map[string]bool{},
		pressured: map[string]bool{},
		cpuUsage:  map[string]int64{},
	}
	for _, row := range restartedRows {
		signals.restarted[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = true
	}
	pressuredRows, err := s.platform.MemoryPressuredInstances(ctx, now.Add(-platformHealthRange))
	if err != nil {
		return dto.PlatformOverviewView{}, err
	}
	for _, row := range pressuredRows {
		signals.pressured[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = true
	}
	cpuRows, err := s.platform.LatestCPUUsage(ctx, now.Add(-retention))
	if err != nil {
		return dto.PlatformOverviewView{}, err
	}
	for _, row := range cpuRows {
		signals.cpuUsage[platformInstanceKey(row.Component, row.Instance, row.ClusterID)] = row.CPUUsageMicros
	}
	events, err := s.platform.LatestLeadershipEvents(ctx, now.Add(-dbrepo.PlatformLeadershipRetention))
	if err != nil {
		return dto.PlatformOverviewView{}, err
	}
	lastChanges := map[string]time.Time{}
	for _, event := range events {
		key := platformComponentKey(event.Component, event.ClusterID)
		if _, exists := lastChanges[key]; !exists {
			lastChanges[key] = event.ChangedAt
		}
	}

	grouped := groupPlatformSamples(samples)
	result := dto.PlatformOverviewView{
		Enabled:        true,
		PlatformHealth: "ok",
		GatewayPlane:   make([]dto.PlatformComponentView, 0, len(gatewayPlaneOrder)),
	}
	for _, component := range gatewayPlaneOrder {
		item := buildPlatformComponent(component, grouped[platformComponentKey(string(component), 0)], lastChanges[platformComponentKey(string(component), 0)], signals, now)
		result.GatewayPlane = append(result.GatewayPlane, item)
		result.PlatformHealth = worsePlatformHealth(result.PlatformHealth, item.Health)
	}

	clusters, err := s.clusters.List(ctx)
	if err != nil {
		return dto.PlatformOverviewView{}, err
	}
	filter := strings.TrimSpace(p.Health)
	all := make([]dto.PlatformClusterView, 0, len(clusters))
	for _, cluster := range clusters {
		components := make([]dto.PlatformComponentView, 0, len(memberPlaneOrder))
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
		all = append(all, dto.PlatformClusterView{
			Name: cluster.Name, Phase: cluster.Phase,
			Health: health, Components: components,
		})
	}
	result.MemberPlane = dto.PlatformMemberPlaneView{
		Total: len(all), Page: page, PageSize: pageSize, Clusters: []dto.PlatformClusterView{},
	}
	start := (page - 1) * pageSize
	if start < len(all) {
		end := min(start+pageSize, len(all))
		result.MemberPlane.Clusters = all[start:end]
	}
	return result, nil
}

func (s *PlatformService) InstanceTrend(ctx context.Context, p dto.PlatformTrendOptions) (dto.PlatformTrendView, error) {
	if s.platform == nil || s.clusters == nil {
		return dto.PlatformTrendView{}, errors.New("platform observability is unavailable")
	}
	from, to, err := s.platformWindow(p.From, p.To, dbrepo.PlatformSampleRetentionMax, 24*time.Hour)
	if err != nil {
		return dto.PlatformTrendView{}, err
	}
	clusterID := int64(0)
	if p.Cluster != "" {
		cluster, err := s.clusters.GetByName(ctx, p.Cluster)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return dto.PlatformTrendView{}, ErrNotFound
			}
			return dto.PlatformTrendView{}, err
		}
		clusterID = cluster.ID
	}
	step := observationTrendStep(to.Sub(from))
	rows, err := s.platform.Trend(ctx, p.Component, p.Instance, clusterID, from, to, step)
	if err != nil {
		return dto.PlatformTrendView{}, err
	}
	items := make([]dto.PlatformTrendPointView, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.PlatformTrendPointView{
			BucketStartedAt: row.BucketStartedAt,
			MemoryBytes:     int(row.MemoryBytes),
			CPUUsageMicros:  int(row.CPUUsageMicros),
			Goroutines:      int(row.Goroutines),
			GCPauseMicros:   int(row.GCPauseMicros),
		})
	}
	return dto.PlatformTrendView{Items: items, StepSeconds: int(step / time.Second), From: from, To: to}, nil
}

func (s *PlatformService) LeadershipEvents(ctx context.Context, p dto.PlatformLeadershipOptions) (dto.PlatformLeadershipPageView, error) {
	if s.platform == nil || s.clusters == nil {
		return dto.PlatformLeadershipPageView{}, errors.New("platform observability is unavailable")
	}
	page, pageSize := p.Page, p.PageSize
	from, to, err := s.platformWindow(p.From, p.To, dbrepo.PlatformLeadershipRetention, 24*time.Hour)
	if err != nil {
		return dto.PlatformLeadershipPageView{}, err
	}
	var clusterID *int64
	if p.Cluster != "" {
		cluster, err := s.clusters.GetByName(ctx, p.Cluster)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return dto.PlatformLeadershipPageView{}, ErrNotFound
			}
			return dto.PlatformLeadershipPageView{}, err
		}
		clusterID = &cluster.ID
	}
	component := p.Component
	result, err := s.platform.LeadershipEvents(ctx, component, clusterID, from, to, page, pageSize)
	if err != nil {
		return dto.PlatformLeadershipPageView{}, err
	}
	items := make([]dto.PlatformLeadershipEventView, 0, len(result.Items))
	for _, event := range result.Items {
		items = append(items, dto.PlatformLeadershipEventView{
			ID: strconv.FormatInt(event.ID, 10), Component: event.Component,
			ClusterID: strconv.FormatInt(event.ClusterID, 10),
			Instance:  event.Instance, ClusterName: event.ClusterName,
			RoleFrom: event.RoleFrom, RoleTo: event.RoleTo,
			ChangedAt: event.ChangedAt, DetectedBy: event.DetectedBy, CreatedAt: event.CreatedAt,
		})
	}
	return dto.PlatformLeadershipPageView{
		Items: items, Total: int(result.Total), Page: page,
		PageSize: pageSize,
	}, nil
}

func (s *PlatformService) Settings(ctx context.Context) (dto.PlatformSettingsView, error) {
	if s.platform == nil {
		return dto.PlatformSettingsView{}, errors.New("platform observability is unavailable")
	}
	settings, err := s.platform.GetSettings(ctx)
	if err != nil {
		return dto.PlatformSettingsView{}, err
	}
	return platformSettingsOf(settings), nil
}

func (s *PlatformService) UpdateSettings(ctx context.Context, request dto.PlatformSettingsInput) (dto.PlatformSettingsView, error) {
	if s.platform == nil {
		return dto.PlatformSettingsView{}, errors.New("platform observability is unavailable")
	}
	maximum := int(dbrepo.PlatformSampleRetentionMax / (24 * time.Hour))
	if request.RetentionDays < 1 || request.RetentionDays > maximum {
		return dto.PlatformSettingsView{}, &ValidationError{Msg: fmt.Sprintf("retentionDays must be between 1 and %d", maximum)}
	}
	if err := s.platform.UpdateSettings(ctx, request.Enabled, request.RetentionDays); err != nil {
		return dto.PlatformSettingsView{}, err
	}
	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, gatewaypb.PlatformObservabilitySettingsTopic)
	}
	return s.Settings(ctx)
}

func platformSettingsOf(settings *dbrepo.PlatformObservabilitySettings) dto.PlatformSettingsView {
	return dto.PlatformSettingsView{
		Enabled: settings.Enabled, RetentionDays: settings.RetentionDays,
		MaxRetentionDays: int(dbrepo.PlatformSampleRetentionMax / (24 * time.Hour)),
	}
}

func (s *PlatformService) platformWindow(fromValue, toValue *time.Time, retention, defaultSpan time.Duration) (time.Time, time.Time, error) {
	now := s.now().UTC()
	to := now
	from := now.Add(-defaultSpan)
	if toValue != nil {
		to = *toValue
	}
	if fromValue != nil {
		from = *fromValue
	}
	if to.After(now) {
		to = now
	}
	if cutoff := now.Add(-retention); from.Before(cutoff) {
		from = cutoff
	}
	from, to = from.UTC(), to.UTC()
	if !from.Before(to) {
		return time.Time{}, time.Time{}, &ValidationError{Msg: "from must be before to within the retention period"}
	}
	return from, to, nil
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

type platformHealthSignals struct {
	restarted map[string]bool
	pressured map[string]bool
	cpuUsage  map[string]int64
}

func buildPlatformComponent(component platformobs.Component, rows []dbrepo.PlatformComponentSample, lastChange time.Time, signals platformHealthSignals, now time.Time) dto.PlatformComponentView {
	active := make([]dbrepo.PlatformComponentSample, 0, len(rows))
	for _, row := range rows {
		if now.Sub(row.LastObservedAt) <= platformNoDataAfter {
			active = append(active, row)
		}
	}
	if len(active) > 0 {
		rows = active
	}

	hasLeadership := component == platformobs.ComponentGateway || component == platformobs.ComponentOperator
	result := dto.PlatformComponentView{
		Component: string(component), Health: "unavailable",
		HasLeadership: hasLeadership, Instances: make([]dto.PlatformInstanceView, 0, len(rows)),
	}
	if !lastChange.IsZero() {
		result.LastLeadershipChange = &lastChange
	}
	leaderCount, noDataCount := 0, 0
	for _, row := range rows {
		key := platformInstanceKey(row.Component, row.Instance, row.ClusterID)
		health, reason := row.Health, row.HealthReason
		switch {
		case now.Sub(row.LastObservedAt) > platformNoDataAfter:
			health, reason = "no_data", "sample_stale"
			if component == platformobs.ComponentConsoleWeb {
				reason = "sample_stale_collected_by_console_api"
			}
			noDataCount++
		case signals.pressured[key]:
			health, reason = "degraded", "memory_pressure_events"
		case signals.restarted[key]:
			health, reason = "degraded", "restarted_in_health_window"
		}
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
		instance := dto.PlatformInstanceView{
			Instance: row.Instance, Role: row.Role, Health: health,
			HealthReason: reason, UptimeSeconds: int(uptime), RestartCount: int(row.RestartCount),
			MemoryBytes: int(row.MemoryBytes), MemoryLimitBytes: int(row.MemoryLimitBytes), CPUMillis: int(cpuMillis),
			Goroutines: int(row.Goroutines), HeapAllocBytes: int(row.HeapAllocBytes), HeapSysBytes: int(row.HeapSysBytes),
			LastObservedAt: row.LastObservedAt,
		}
		if stats, ok := databaseStatsOf(dbrepo.DecodeDBStats(row)); ok {
			instance.DBStats = &stats
		}
		result.Instances = append(result.Instances, instance)
	}
	sort.SliceStable(result.Instances, func(i, j int) bool {
		left, right := result.Instances[i], result.Instances[j]
		if rank := platformRoleRank(string(left.Role)) - platformRoleRank(string(right.Role)); rank != 0 {
			return rank < 0
		}
		return left.Instance < right.Instance
	})
	switch {
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
				if result.HealthReason == "" {
					result.HealthReason = instance.HealthReason
				}
			}
		}
	}
	if hasLeadership && result.Health != "unavailable" && result.Health != "pending" && leaderCount != 1 {
		result.Health = "degraded"
		result.NoLeader = true
		result.HealthReason = "no_leader"
	}
	if result.Health == "degraded" && result.HealthReason == "" {
		result.HealthReason = "unspecified"
	}
	result.SingleReplica = hasLeadership && len(result.Instances) == 1 && !result.NoLeader
	return result
}

func databaseStatsOf(values map[string]any) (dto.PlatformDatabaseStatsView, bool) {
	if len(values) == 0 {
		return dto.PlatformDatabaseStatsView{}, false
	}
	data, err := json.Marshal(values)
	if err != nil {
		return dto.PlatformDatabaseStatsView{}, false
	}
	var stats dto.PlatformDatabaseStatsView
	if err := json.Unmarshal(data, &stats); err != nil {
		return dto.PlatformDatabaseStatsView{}, false
	}
	return stats, true
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

func worsePlatformHealth(current, candidate string) string {
	rank := map[string]int{"ok": 0, "pending": 0, "degraded": 1, "unavailable": 2}
	if rank[candidate] > rank[current] {
		return candidate
	}
	return current
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
