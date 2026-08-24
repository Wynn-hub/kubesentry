package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

type fakePlatformRepository struct {
	settings dbrepo.PlatformObservabilitySettings
	updated  bool
}

func (f *fakePlatformRepository) GetSettings(context.Context) (*dbrepo.PlatformObservabilitySettings, error) {
	settings := f.settings
	return &settings, nil
}

func (f *fakePlatformRepository) UpdateSettings(_ context.Context, enabled bool, retentionDays int) error {
	f.settings.Enabled = enabled
	f.settings.RetentionDays = retentionDays
	f.updated = true
	return nil
}

func (*fakePlatformRepository) LatestSamples(context.Context, time.Time) ([]dbrepo.PlatformComponentSample, error) {
	return nil, nil
}

func (*fakePlatformRepository) RestartedInstances(context.Context, time.Time) ([]dbrepo.PlatformRestartedInstance, error) {
	return nil, nil
}

func (*fakePlatformRepository) MemoryPressuredInstances(context.Context, time.Time) ([]dbrepo.PlatformRestartedInstance, error) {
	return nil, nil
}

func (*fakePlatformRepository) LatestCPUUsage(context.Context, time.Time) ([]dbrepo.PlatformCPUUsage, error) {
	return nil, nil
}

func (*fakePlatformRepository) Trend(context.Context, string, string, int64, time.Time, time.Time, time.Duration) ([]dbrepo.PlatformTrendRow, error) {
	return nil, nil
}

func (*fakePlatformRepository) LatestLeadershipEvents(context.Context, time.Time) ([]dbrepo.PlatformLeadershipEvent, error) {
	return nil, nil
}

func (*fakePlatformRepository) LeadershipEvents(context.Context, string, *int64, time.Time, time.Time, int, int) (dbrepo.LeadershipEventPage, error) {
	return dbrepo.LeadershipEventPage{}, nil
}

type fakePlatformClusterRepository struct{}

func (fakePlatformClusterRepository) List(context.Context) ([]dbrepo.Cluster, error) {
	return nil, nil
}

func (fakePlatformClusterRepository) GetByName(context.Context, string) (*dbrepo.Cluster, error) {
	return nil, nil
}

func TestPlatformServiceUsesTypedRepositoriesForSettings(t *testing.T) {
	repo := &fakePlatformRepository{settings: dbrepo.PlatformObservabilitySettings{Enabled: false, RetentionDays: 1}}
	notifier := &recordingNotifier{}
	svc := NewPlatformService(repo, fakePlatformClusterRepository{}, notifier)

	settings, err := svc.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || settings.RetentionDays != 1 {
		t.Fatalf("settings = %+v", settings)
	}

	updated, err := svc.UpdateSettings(context.Background(), &dto.PlatformSettingsRequest{
		Enabled: true, RetentionDays: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.updated || !updated.Enabled || updated.RetentionDays != 3 {
		t.Fatalf("updated settings = %+v, repo updated = %v", updated, repo.updated)
	}
	if len(notifier.topics) != 1 {
		t.Fatalf("notifications = %v", notifier.topics)
	}
}

func TestPlatformServiceOverviewPreservesDisabledState(t *testing.T) {
	repo := &fakePlatformRepository{settings: dbrepo.PlatformObservabilitySettings{Enabled: false, RetentionDays: 1}}
	svc := NewPlatformService(repo, fakePlatformClusterRepository{}, nil)

	overview, err := svc.Overview(context.Background(), dto.GetPlatformOverviewParams{})
	if err != nil {
		t.Fatal(err)
	}
	if overview.Enabled || overview.PlatformHealth != dto.PlatformOverviewHealthDisabled {
		t.Fatalf("overview = %+v", overview)
	}
	if overview.GatewayPlane == nil || overview.MemberPlane.Clusters == nil {
		t.Fatalf("disabled collections must be empty arrays: %+v", overview)
	}
}

func TestBuildPlatformComponentPreservesHealthSemantics(t *testing.T) {
	now := time.Now().UTC()
	quiet := platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}}
	row := dbrepo.PlatformComponentSample{
		Component: string(platformobs.ComponentOperator), Instance: "operator-0",
		Role: string(platformobs.RoleLeader), ProcessStartedAt: now.Add(-time.Hour),
		MemoryBytes: 127 * 1024 * 1024, MemoryLimitBytes: 128 * 1024 * 1024,
		Health: string(platformobs.HealthOK), LastObservedAt: now,
	}

	healthy := buildPlatformComponent(platformobs.ComponentOperator, []dbrepo.PlatformComponentSample{row}, time.Time{}, quiet, now)
	if healthy.Health != dto.PlatformComponentHealthOk || !healthy.SingleReplica || healthy.NoLeader {
		t.Fatalf("healthy single leader = %+v", healthy)
	}

	standby := row
	standby.Role = string(platformobs.RoleStandby)
	leaderless := buildPlatformComponent(platformobs.ComponentOperator, []dbrepo.PlatformComponentSample{standby}, time.Time{}, quiet, now)
	if leaderless.Health != dto.PlatformComponentHealthDegraded || !leaderless.NoLeader || leaderless.HealthReason != "no_leader" {
		t.Fatalf("leaderless component = %+v", leaderless)
	}

	stale := row
	stale.Component = string(platformobs.ComponentConsoleWeb)
	stale.Role = string(platformobs.RoleNone)
	stale.LastObservedAt = now.Add(-platformNoDataAfter - time.Second)
	stopped := buildPlatformComponent(platformobs.ComponentConsoleWeb, []dbrepo.PlatformComponentSample{stale}, time.Time{}, quiet, now)
	if stopped.Health != dto.PlatformComponentHealthUnavailable || stopped.Instances[0].HealthReason != "sample_stale_collected_by_console_api" {
		t.Fatalf("stale component = %+v", stopped)
	}

	pending := buildPlatformComponent(platformobs.ComponentOperator, nil, time.Time{}, quiet, now)
	if pending.Health != dto.PlatformComponentHealthPending || pending.NoLeader || worsePlatformHealth("ok", string(pending.Health)) != "ok" {
		t.Fatalf("pending component = %+v", pending)
	}
}

func TestBuildPlatformComponentUsesPressureAndRestartSignals(t *testing.T) {
	now := time.Now().UTC()
	row := dbrepo.PlatformComponentSample{
		Component: string(platformobs.ComponentConsoleAPI), Instance: "console-0",
		Role: string(platformobs.RoleNone), ProcessStartedAt: now.Add(-time.Hour),
		MemoryBytes: 127 * 1024 * 1024, MemoryLimitBytes: 128 * 1024 * 1024,
		Health: string(platformobs.HealthOK), LastObservedAt: now,
	}
	key := platformInstanceKey(row.Component, row.Instance, row.ClusterID)

	quiet := platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}}
	component := buildPlatformComponent(platformobs.ComponentConsoleAPI, []dbrepo.PlatformComponentSample{row}, time.Time{}, quiet, now)
	if component.Health != dto.PlatformComponentHealthOk {
		t.Fatalf("high memory ratio without pressure = %+v", component)
	}

	pressured := platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{key: true}, cpuUsage: map[string]int64{}}
	component = buildPlatformComponent(platformobs.ComponentConsoleAPI, []dbrepo.PlatformComponentSample{row}, time.Time{}, pressured, now)
	if component.Health != dto.PlatformComponentHealthDegraded || component.Instances[0].HealthReason != "memory_pressure_events" {
		t.Fatalf("pressured component = %+v", component)
	}

	restarted := platformHealthSignals{restarted: map[string]bool{key: true}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}}
	component = buildPlatformComponent(platformobs.ComponentConsoleAPI, []dbrepo.PlatformComponentSample{row}, time.Time{}, restarted, now)
	if component.Health != dto.PlatformComponentHealthDegraded || component.Instances[0].HealthReason != "restarted_in_health_window" {
		t.Fatalf("restarted component = %+v", component)
	}
}

func TestBuildPlatformComponentSortsLeaderFirst(t *testing.T) {
	now := time.Now().UTC()
	row := func(instance string, role platformobs.Role) dbrepo.PlatformComponentSample {
		return dbrepo.PlatformComponentSample{
			Component: string(platformobs.ComponentGateway), Instance: instance, Role: string(role),
			ProcessStartedAt: now.Add(-time.Hour), Health: string(platformobs.HealthOK), LastObservedAt: now,
		}
	}
	quiet := platformHealthSignals{restarted: map[string]bool{}, pressured: map[string]bool{}, cpuUsage: map[string]int64{}}
	component := buildPlatformComponent(platformobs.ComponentGateway, []dbrepo.PlatformComponentSample{
		row("standby-b", platformobs.RoleStandby), row("leader", platformobs.RoleLeader), row("standby-a", platformobs.RoleStandby),
	}, time.Time{}, quiet, now)
	got := []string{component.Instances[0].Instance, component.Instances[1].Instance, component.Instances[2].Instance}
	want := []string{"leader", "standby-a", "standby-b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("instance order = %v", got)
		}
	}
}
