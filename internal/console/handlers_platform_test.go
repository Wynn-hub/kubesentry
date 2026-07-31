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
	if rec.Code != 200 || !env.Success {
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
	if rec.Code != 200 || !env.Success {
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
		map[string]any{"admissionPipelineEnabled": true})
	if rec.Code != 200 || !env.Success {
		t.Fatalf("settings = %d: %s", rec.Code, rec.Body.String())
	}
	settings := mustUnmarshal[platformSettingsResponse](t, env.Data)
	if !settings.AdmissionPipelineEnabled {
		t.Fatalf("settings = %+v", settings)
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
		map[string]bool{},
		map[string]int64{},
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
		map[string]bool{},
		map[string]int64{},
		now,
	)
	if component.Health != "degraded" {
		t.Fatalf("leaderless operator health = %q", component.Health)
	}

	restarted := map[string]bool{platformInstanceKey(row.Component, row.Instance, row.ClusterID): true}
	component = buildPlatformComponent(
		platformobs.ComponentConsoleAPI,
		[]dbrepo.PlatformComponentSample{row},
		time.Time{},
		restarted,
		map[string]int64{},
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
		map[string]bool{},
		map[string]int64{},
		now,
	)
	if component.Health != "unavailable" ||
		component.Instances[0].HealthReason != "sample_stale_collected_by_console_api" {
		t.Fatalf("stale console web component = %+v", component)
	}
}
