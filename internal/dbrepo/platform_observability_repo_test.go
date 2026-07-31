package dbrepo_test

import (
	"context"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

func platformSample(now time.Time) platformobs.Sample {
	return platformobs.Sample{
		Component:          platformobs.ComponentOperator,
		Instance:           "operator-0",
		ClusterID:          42,
		ClusterName:        "member-a",
		ObservedAt:         now,
		Role:               platformobs.RoleStandby,
		ProcessStartedAt:   now.Add(-time.Hour),
		MemoryBytes:        100,
		MemoryLimitBytes:   1000,
		CPUUsageMicros:     200,
		CPUQuotaMicros:     50000,
		Goroutines:         10,
		HeapAllocBytes:     50,
		HeapSysBytes:       80,
		GCPauseTotalMicros: 4,
		GCCount:            2,
		Health:             platformobs.HealthOK,
	}
}

func TestPlatformObservabilityUpsertKeepsLatestAndDerivesRestart(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPlatformObservabilityRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)

	first := platformSample(now)
	result, err := repo.UpsertSample(ctx, first, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.RestartCount != 0 || result.RoleChanged {
		t.Fatalf("first result = %+v", result)
	}

	latest := first
	latest.MemoryBytes = 300
	latest.CPUUsageMicros = 500
	latest.ObservedAt = now.Add(20 * time.Second)
	if _, err := repo.UpsertSample(ctx, latest, now.Add(20*time.Second)); err != nil {
		t.Fatal(err)
	}

	restarted := latest
	restarted.ObservedAt = now.Add(time.Minute)
	restarted.ProcessStartedAt = now.Add(30 * time.Second)
	restarted.Role = platformobs.RoleLeader
	result, err = repo.UpsertSample(ctx, restarted, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.RestartCount != 1 || !result.RoleChanged {
		t.Fatalf("restart result = %+v", result)
	}
	restartedRows, err := repo.RestartedInstances(ctx, now.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(restartedRows) != 1 || restartedRows[0].Instance != first.Instance {
		t.Fatalf("restarted instances = %+v", restartedRows)
	}

	rows, err := repo.LatestSamples(ctx, now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].MemoryBytes != 300 || rows[0].RestartCount != 1 ||
		rows[0].Role != string(platformobs.RoleLeader) {
		t.Fatalf("latest rows = %+v", rows)
	}

	events, err := repo.LeadershipEvents(ctx, "operator", &first.ClusterID,
		now.Add(-time.Minute), now.Add(2*time.Minute), 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if events.Total != 1 || len(events.Items) != 1 ||
		events.Items[0].RoleFrom != "standby" || events.Items[0].RoleTo != "leader" {
		t.Fatalf("leadership events = %+v", events)
	}
}

func TestPlatformObservabilitySameProcessDoesNotIncrementRestart(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPlatformObservabilityRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	sample := platformSample(now)
	if _, err := repo.UpsertSample(ctx, sample, now); err != nil {
		t.Fatal(err)
	}
	sample.ObservedAt = now.Add(time.Minute)
	result, err := repo.UpsertSample(ctx, sample, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.RestartCount != 0 {
		t.Fatalf("restart count = %d, want 0", result.RestartCount)
	}
}

func TestPlatformObservabilityCleanupUsesIndependentRetention(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPlatformObservabilityRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)

	oldSample := platformSample(now.Add(-8 * 24 * time.Hour))
	if _, err := repo.UpsertSample(ctx, oldSample, oldSample.ObservedAt); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordSessionLost(ctx, platformobs.ComponentOperator,
		oldSample.ClusterID, oldSample.ClusterName, oldSample.Instance,
		now.Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	samples, events, err := repo.Cleanup(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if samples != 1 || events != 1 {
		t.Fatalf("cleanup = (%d, %d), want (1, 1)", samples, events)
	}
	settings, err := repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.LastCleanupAt == nil {
		t.Fatal("last cleanup time was not recorded")
	}
}

func TestPlatformObservabilitySettingsDefaultDisabled(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPlatformObservabilityRepo(db)
	ctx := context.Background()
	settings, err := repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.AdmissionPipelineEnabled {
		t.Fatal("admission pipeline observation should default to disabled")
	}
	if err := repo.SetAdmissionPipelineEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	settings, err = repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.AdmissionPipelineEnabled {
		t.Fatal("admission pipeline observation was not enabled")
	}
}
