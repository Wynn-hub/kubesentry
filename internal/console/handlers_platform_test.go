package console

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
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
	overview := mustUnmarshal[dto.PlatformOverviewView](t, env.Data)
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
	rec, env := doRequest(t, server, "GET", "/api/v1/platform/instances/gateway/gateway-0/trend?from="+from+"&to="+to, nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("trend = %d: %s", rec.Code, rec.Body.String())
	}
	trend := mustUnmarshal[dto.PlatformTrendView](t, env.Data)
	if len(trend.Items) != 2 || trend.Items[1].CPUUsageMicros != 2000 || trend.Items[1].GCPauseMicros != 60 {
		t.Fatalf("trend = %+v", trend.Items)
	}

	rec, env = doRequest(t, server, "PUT", "/api/v1/platform/settings", map[string]any{"enabled": true, "retentionDays": 3})
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("settings = %d: %s", rec.Code, rec.Body.String())
	}
	settings := mustUnmarshal[dto.PlatformSettingsView](t, env.Data)
	if !settings.Enabled || settings.RetentionDays != 3 || settings.MaxRetentionDays != 7 {
		t.Fatalf("settings = %+v", settings)
	}
	if rec, _ := doRequest(t, server, "PUT", "/api/v1/platform/settings", map[string]any{"enabled": true, "retentionDays": 30}); rec.Code != 400 {
		t.Fatalf("retention above the maximum returned %d, want 400", rec.Code)
	}
}

func TestPlatformOverviewReportsDisabledState(t *testing.T) {
	server, _ := newDBTestServer(t)
	rec, env := doRequest(t, server, "GET", "/api/v1/platform/overview?page=1&pageSize=20", nil)
	if rec.Code != 200 || env.Code != 0 {
		t.Fatalf("overview = %d: %s", rec.Code, rec.Body.String())
	}
	overview := mustUnmarshal[dto.PlatformOverviewView](t, env.Data)
	if overview.Enabled || overview.PlatformHealth != "disabled" || len(overview.GatewayPlane) != 0 {
		t.Fatalf("disabled overview = %+v", overview)
	}
}
