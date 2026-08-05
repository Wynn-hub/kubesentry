package console

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

func TestPlatformOverviewSeparatesGatewayAndMemberHealth(t *testing.T) {
	server, repos := newDBTestServer(t)
	ctx := context.Background()
	if err := repos.Platform.UpdateSettings(ctx, true, 1); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	cluster := &dbrepo.Cluster{Name: "member-a", Phase: "Online"}
	if err := repos.Clusters.Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	write := func(sample platformobs.Sample) {
		t.Helper()
		if _, err := repos.Platform.UpsertSample(ctx, sample, now); err != nil {
			t.Fatal(err)
		}
	}
	sample := func(component platformobs.Component, instance string, clusterID int64, role platformobs.Role) platformobs.Sample {
		value := platformobs.UnavailableSample(component, instance, now)
		value.ClusterID = clusterID
		value.ClusterName = cluster.Name
		value.Role = role
		value.ProcessStartedAt = now.Add(-time.Hour)
		value.MemoryBytes = 100
		value.MemoryLimitBytes = 1000
		return value
	}
	write(sample(platformobs.ComponentGateway, "gateway-0", 0, platformobs.RoleLeader))
	write(sample(platformobs.ComponentConsoleAPI, "console-0", 0, platformobs.RoleNone))
	write(sample(platformobs.ComponentConsoleWeb, "console-0", 0, platformobs.RoleNone))
	postgres := sample(platformobs.ComponentPostgres, "postgres", 0, platformobs.RoleNone)
	postgres.DBStats = map[string]any{"connections": 2}
	write(postgres)
	write(sample(platformobs.ComponentWebhook, "webhook-0", cluster.ID, platformobs.RoleNone))
	write(sample(platformobs.ComponentOperator, "operator-0", cluster.ID, platformobs.RoleStandby))

	rec, env := doRequest(t, server, "GET", "/api/v1/platform/overview?page=1&pageSize=20", nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("overview = %d: %s", rec.Code, rec.Body.String())
	}
	overview := mustUnmarshal[platformOverviewResponse](t, env.Data)
	if overview.PlatformHealth != "ok" || len(overview.GatewayPlane) != 4 {
		t.Fatalf("gateway plane = %+v", overview)
	}
	if overview.MemberPlane.Total != 1 || len(overview.MemberPlane.Clusters) != 1 ||
		overview.MemberPlane.Clusters[0].Health != "degraded" {
		t.Fatalf("member plane = %+v", overview.MemberPlane)
	}
}

func TestPlatformSettingsAndTrend(t *testing.T) {
	server, repos := newDBTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Minute)
	first := platformobs.UnavailableSample(platformobs.ComponentGateway, "gateway-0", now)
	first.Role = platformobs.RoleLeader
	first.ProcessStartedAt = now.Add(-time.Hour)
	first.MemoryBytes = 100
	first.CPUUsageMicros = 1000
	first.Goroutines = 10
	first.GCPauseTotalMicros = 100
	if _, err := repos.Platform.UpsertSample(ctx, first, now); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ObservedAt = now.Add(time.Minute)
	second.MemoryBytes = 200
	second.CPUUsageMicros = 3000
	second.GCPauseTotalMicros = 160
	if _, err := repos.Platform.UpsertSample(ctx, second, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	from := url.QueryEscape(now.Add(-time.Minute).Format(time.RFC3339))
	to := url.QueryEscape(now.Add(2 * time.Minute).Format(time.RFC3339))
	rec, env := doRequest(t, server, "GET",
		"/api/v1/platform/instances/gateway/gateway-0/trend?from="+from+"&to="+to, nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("trend = %d: %s", rec.Code, rec.Body.String())
	}
	trend := mustUnmarshal[struct {
		Items []dbrepo.PlatformTrendRow `json:"items"`
	}](t, env.Data)
	if len(trend.Items) != 2 || trend.Items[1].CPUUsageMicros != 2000 ||
		trend.Items[1].GCPauseMicros != 60 {
		t.Fatalf("trend = %+v", trend.Items)
	}

	rec, env = doRequest(t, server, "PUT", "/api/v1/platform/settings",
		map[string]any{"enabled": true, "retentionDays": 3})
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("settings = %d: %s", rec.Code, rec.Body.String())
	}
	settings := mustUnmarshal[platformSettingsResponse](t, env.Data)
	if !settings.Enabled || settings.RetentionDays != 3 || settings.MaxRetentionDays != 7 {
		t.Fatalf("settings = %+v", settings)
	}
	if rec, _ := doRequest(t, server, "PUT", "/api/v1/platform/settings",
		map[string]any{"enabled": true, "retentionDays": 30}); rec.Code != 400 {
		t.Fatalf("retention above the maximum returned %d, want 400", rec.Code)
	}
}

// The overview must distinguish "switched off" from "on but nothing is
// reporting" — both are empty in the data, and only one needs investigating.
func TestPlatformOverviewReportsDisabledState(t *testing.T) {
	server, _ := newDBTestServer(t)
	rec, env := doRequest(t, server, "GET", "/api/v1/platform/overview?page=1&pageSize=20", nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("overview = %d: %s", rec.Code, rec.Body.String())
	}
	overview := mustUnmarshal[platformOverviewResponse](t, env.Data)
	if overview.Enabled || overview.PlatformHealth != "disabled" || len(overview.GatewayPlane) != 0 {
		t.Fatalf("disabled overview = %+v", overview)
	}
}

func TestPlatformComponentHealthHandlesUnavailableLimitsLeadershipAndRestarts(t *testing.T) {
	now := time.Now().UTC()
	row := dbrepo.PlatformComponentSample{
		Component:        string(platformobs.ComponentConsoleAPI),
		Instance:         "console-0",
		Role:             string(platformobs.RoleNone),
		ProcessStartedAt: now.Add(-time.Hour),
		MemoryBytes:      -1,
		MemoryLimitBytes: -1,
		Health:           string(platformobs.HealthOK),
		LastObservedAt:   now,
	}
	component := buildPlatformComponent(
		platformobs.ComponentConsoleAPI,
		[]dbrepo.PlatformComponentSample{row},
		time.Time{},
		platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}},
		now,
	)
	if component.Health != "ok" || component.Instances[0].CPUMillis != -1 {
		t.Fatalf("component = %+v", component)
	}

	operator := row
	operator.Component = string(platformobs.ComponentOperator)
	operator.Role = string(platformobs.RoleStandby)
	component = buildPlatformComponent(
		platformobs.ComponentOperator,
		[]dbrepo.PlatformComponentSample{operator},
		time.Time{},
		platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}},
		now,
	)
	if component.Health != "degraded" {
		t.Fatalf("leaderless operator health = %q", component.Health)
	}

	restarted := platformHealthSignals{
		restarted: map[string]bool{platformInstanceKey(row.Component, row.Instance, row.ClusterID): true},
		pressured: map[string]bool{}, cpuUsage: map[string]int64{},
	}
	component = buildPlatformComponent(
		platformobs.ComponentConsoleAPI,
		[]dbrepo.PlatformComponentSample{row},
		time.Time{},
		restarted,
		now,
	)
	if component.Health != "degraded" || component.Instances[0].HealthReason != "restarted_in_health_window" {
		t.Fatalf("restarted component = %+v", component)
	}

	web := row
	web.Component = string(platformobs.ComponentConsoleWeb)
	web.LastObservedAt = now.Add(-platformNoDataAfter - time.Second)
	component = buildPlatformComponent(
		platformobs.ComponentConsoleWeb,
		[]dbrepo.PlatformComponentSample{web},
		time.Time{},
		platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}},
		now,
	)
	if component.Health != "unavailable" ||
		component.Instances[0].HealthReason != "sample_stale_collected_by_console_api" {
		t.Fatalf("stale console web component = %+v", component)
	}
}

// A Go process grows its RSS toward the cgroup limit before GC reclaims, so a
// high usage ratio is ordinary operation. Judging on the ratio marked every
// healthy console-api degraded (its limit is 128Mi); only the kernel's own
// verdict — an allocation that had to reclaim at the ceiling, or an OOM kill —
// means anything.
func TestPlatformHealthIgnoresHighMemoryRatioWithoutPressureEvents(t *testing.T) {
	now := time.Now().UTC()
	row := dbrepo.PlatformComponentSample{
		Component:        string(platformobs.ComponentConsoleAPI),
		Instance:         "console-0",
		Role:             string(platformobs.RoleNone),
		ProcessStartedAt: now.Add(-time.Hour),
		MemoryBytes:      127 * 1024 * 1024, // 99% of the limit
		MemoryLimitBytes: 128 * 1024 * 1024,
		Health:           string(platformobs.HealthOK),
		LastObservedAt:   now,
	}
	quiet := platformHealthSignals{
		restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{},
	}
	component := buildPlatformComponent(
		platformobs.ComponentConsoleAPI, []dbrepo.PlatformComponentSample{row}, time.Time{}, quiet, now)
	if component.Health != "ok" {
		t.Fatalf("health = %q at 99%% of limit with no pressure events, want ok", component.Health)
	}

	pressured := quiet
	pressured.pressured = map[string]bool{
		platformInstanceKey(row.Component, row.Instance, row.ClusterID): true,
	}
	component = buildPlatformComponent(
		platformobs.ComponentConsoleAPI, []dbrepo.PlatformComponentSample{row}, time.Time{}, pressured, now)
	if component.Health != "degraded" || component.Instances[0].HealthReason != "memory_pressure_events" {
		t.Fatalf("component with pressure events = %+v", component)
	}
}

// A lone leader is a valid single-replica install; a lone standby means nobody
// holds the lease. Both show one instance, so the response has to distinguish
// them or the UI cannot tell "no secondary deployed" from "secondary is gone".
func TestPlatformSingleReplicaIsNotAMissingSecondary(t *testing.T) {
	now := time.Now().UTC()
	quiet := platformHealthSignals{
		restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{},
	}
	row := func(role platformobs.Role) dbrepo.PlatformComponentSample {
		return dbrepo.PlatformComponentSample{
			Component: string(platformobs.ComponentOperator), Instance: "operator-0", Role: string(role),
			ProcessStartedAt: now.Add(-time.Hour), MemoryLimitBytes: -1,
			Health: string(platformobs.HealthOK), LastObservedAt: now,
		}
	}

	leaderOnly := buildPlatformComponent(platformobs.ComponentOperator,
		[]dbrepo.PlatformComponentSample{row(platformobs.RoleLeader)}, time.Time{}, quiet, now)
	if leaderOnly.Health != "ok" || !leaderOnly.SingleReplica || leaderOnly.NoLeader {
		t.Fatalf("single leader = %+v, want healthy single replica", leaderOnly)
	}

	standbyOnly := buildPlatformComponent(platformobs.ComponentOperator,
		[]dbrepo.PlatformComponentSample{row(platformobs.RoleStandby)}, time.Time{}, quiet, now)
	if standbyOnly.Health != "degraded" || !standbyOnly.NoLeader || standbyOnly.SingleReplica {
		t.Fatalf("lone standby = %+v, want degraded with no leader", standbyOnly)
	}
	if standbyOnly.HealthReason != "no_leader" {
		t.Fatalf("reason = %q, want no_leader", standbyOnly.HealthReason)
	}
}

// An unexplained warning colour gives the operator nothing to act on, so every
// non-ok component must carry a reason the UI can render.
func TestPlatformDegradedComponentsAlwaysStateAReason(t *testing.T) {
	now := time.Now().UTC()
	quiet := platformHealthSignals{
		restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{},
	}
	fresh := dbrepo.PlatformComponentSample{
		Component: string(platformobs.ComponentWebhook), Instance: "webhook-0",
		Role: string(platformobs.RoleNone), ProcessStartedAt: now.Add(-time.Hour),
		MemoryLimitBytes: -1, Health: string(platformobs.HealthOK), LastObservedAt: now,
	}
	stale := fresh
	stale.Instance = "webhook-1"
	stale.LastObservedAt = now.Add(-platformNoDataAfter - time.Second)

	partial := buildPlatformComponent(platformobs.ComponentWebhook,
		[]dbrepo.PlatformComponentSample{fresh, stale}, time.Time{}, quiet, now)
	if partial.Health != "degraded" || partial.HealthReason != "component_partial_no_data" {
		t.Fatalf("partially stale component = %+v", partial)
	}

	// "Never reported" and "reported then stopped" both have no fresh sample,
	// but only the second is a fault. Right after the switch is enabled every
	// component is in the first state, and calling that unavailable raised an
	// alarm about a healthy platform that simply had not reported yet.
	empty := buildPlatformComponent(platformobs.ComponentWebhook, nil, time.Time{}, quiet, now)
	if empty.Health != "pending" || empty.HealthReason != "component_pending" {
		t.Fatalf("component with no samples at all = %+v, want pending", empty)
	}
	if worsePlatformHealth("ok", empty.Health) != "ok" {
		t.Error("a pending component must not raise the platform banner")
	}

	stopped := buildPlatformComponent(platformobs.ComponentWebhook,
		[]dbrepo.PlatformComponentSample{stale}, time.Time{}, quiet, now)
	if stopped.Health != "unavailable" || stopped.HealthReason != "component_no_samples" {
		t.Fatalf("component that stopped reporting = %+v, want unavailable", stopped)
	}
	if worsePlatformHealth("ok", stopped.Health) != "unavailable" {
		t.Error("a component that stopped reporting must raise the platform banner")
	}

	// Leadership is undecidable without samples, so a pending component must not
	// be reported as having lost its leader.
	pendingOperator := buildPlatformComponent(platformobs.ComponentOperator, nil, time.Time{}, quiet, now)
	if pendingOperator.NoLeader || pendingOperator.SingleReplica {
		t.Fatalf("pending leadership component = %+v", pendingOperator)
	}

	// A degraded sample that arrived without a reason still has to say something.
	reasonless := fresh
	reasonless.Health = string(platformobs.HealthDegraded)
	reasonless.HealthReason = ""
	fallback := buildPlatformComponent(platformobs.ComponentWebhook,
		[]dbrepo.PlatformComponentSample{reasonless}, time.Time{}, quiet, now)
	if fallback.Health != "degraded" || fallback.HealthReason != "unspecified" ||
		fallback.Instances[0].HealthReason != "unspecified" {
		t.Fatalf("reasonless degraded component = %+v", fallback)
	}
}

// Sample rows arrive in map-iteration order, so without an explicit sort the
// primary and secondary swap places between polls.
func TestPlatformInstancesSortLeaderFirst(t *testing.T) {
	now := time.Now().UTC()
	instance := func(name string, role platformobs.Role) dbrepo.PlatformComponentSample {
		return dbrepo.PlatformComponentSample{
			Component: string(platformobs.ComponentGateway), Instance: name, Role: string(role),
			ProcessStartedAt: now.Add(-time.Hour), MemoryLimitBytes: -1,
			Health: string(platformobs.HealthOK), LastObservedAt: now,
		}
	}
	component := buildPlatformComponent(platformobs.ComponentGateway,
		[]dbrepo.PlatformComponentSample{
			instance("gateway-9", platformobs.RoleStandby),
			instance("gateway-0", platformobs.RoleLeader),
		},
		time.Time{},
		platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}},
		now)
	if component.Instances[0].Role != string(platformobs.RoleLeader) ||
		component.Instances[1].Role != string(platformobs.RoleStandby) {
		t.Fatalf("instance order = %+v, want leader first", component.Instances)
	}
}
