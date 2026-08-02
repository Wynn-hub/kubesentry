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

// TIMESTAMPTZ keeps microseconds while time.Now() carries nanoseconds, so a
// start time that has been through the database never equals the one the
// process reports. Restart detection is exactly that comparison, so without
// normalization every healthy instance accrued one restart per sample — and
// then showed up as degraded with "restarted in the health window".
func TestPlatformObservabilityNanosecondStartTimeIsNotARestart(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPlatformObservabilityRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)

	sample := platformSample(now)
	sample.ProcessStartedAt = time.Date(2026, 7, 31, 10, 0, 0, 123456789, time.UTC)
	if _, err := repo.UpsertSample(ctx, sample, now); err != nil {
		t.Fatal(err)
	}
	for minute := 1; minute <= 3; minute++ {
		next := sample
		next.ObservedAt = now.Add(time.Duration(minute) * time.Minute)
		result, err := repo.UpsertSample(ctx, next, next.ObservedAt)
		if err != nil {
			t.Fatal(err)
		}
		if result.RestartCount != 0 {
			t.Fatalf("restart count = %d after sample %d of the same process, want 0",
				result.RestartCount, minute)
		}
	}

	// A genuinely different process must still be counted.
	restarted := sample
	restarted.ObservedAt = now.Add(4 * time.Minute)
	restarted.ProcessStartedAt = sample.ProcessStartedAt.Add(time.Second)
	result, err := repo.UpsertSample(ctx, restarted, restarted.ObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	if result.RestartCount != 1 {
		t.Fatalf("restart count = %d after a new process start, want 1", result.RestartCount)
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

// Cleanup pages by primary key, so an expired set larger than one batch must be
// fully removed rather than truncated at the batch boundary.
func TestPlatformObservabilityCleanupPagesBeyondOneBatch(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPlatformObservabilityRepo(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)

	const total = 1200 // > platformCleanupBatchSize
	for i := 0; i < total; i++ {
		// Distinct minute buckets so each sample is its own row rather than an
		// upsert onto the previous one.
		sample := platformSample(now.Add(-8 * 24 * time.Hour).Add(-time.Duration(i) * time.Minute))
		if _, err := repo.UpsertSample(ctx, sample, sample.ObservedAt); err != nil {
			t.Fatalf("seed sample %d: %v", i, err)
		}
	}

	samples, _, err := repo.Cleanup(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if samples != total {
		t.Errorf("deleted = %d, want %d", samples, total)
	}
	var remaining int64
	if err := db.Model(&dbrepo.PlatformComponentSample{}).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("remaining = %d samples, want 0", remaining)
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
	if settings.Enabled {
		t.Fatal("platform observation should default to disabled")
	}
	if settings.RetentionDays != 1 {
		t.Fatalf("default retention = %d days, want 1", settings.RetentionDays)
	}
	if err := repo.UpdateSettings(ctx, true, 3); err != nil {
		t.Fatal(err)
	}
	settings, err = repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled || settings.RetentionDays != 3 {
		t.Fatalf("settings = %+v, want enabled with 3-day retention", settings)
	}
	// The ceiling exists because minute-resolution samples are for reading a
	// failure that just happened, not for capacity planning.
	if err := repo.UpdateSettings(ctx, true, 8); err == nil {
		t.Fatal("retention above the maximum was accepted")
	}
}
