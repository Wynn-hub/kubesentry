package dbrepo_test

import (
	"context"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func observationReport(series string, sequence uint64, observedAt time.Time, generated int64) admissionobs.Report {
	return admissionobs.Report{
		ReportID:   admissionobs.ReportID(series, sequence),
		SeriesID:   series,
		Sequence:   sequence,
		ObservedAt: observedAt,
		Component:  admissionobs.ComponentWebhook,
		Snapshot: admissionobs.Snapshot{
			Counters:             admissionobs.Counters{Generated: generated},
			FaultReasonCounts:    map[string]int64{},
			PressureReasonCounts: map[string]int64{},
			FilterReasonCounts:   map[string]int64{},
		},
		QueueDepthMax: 3,
		QueueCapacity: 256,
	}
}

func TestObservabilityRepoAppliesCumulativeReportsAndSequenceGap(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "observed-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	repo := dbrepo.NewObservabilityRepo(db)
	now := time.Now().UTC().Truncate(time.Second)

	first := observationReport("series-a", 1, now, 4)
	first.Snapshot.FaultReasonCounts[admissionobs.ReasonPostFailed] = 1
	result, err := repo.ApplyMemberReport(ctx, cluster.ID, cluster.Name, first, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.BaselineReset || result.Duplicate || result.SequenceGap != 0 {
		t.Fatalf("unexpected first result: %+v", result)
	}

	third := observationReport("series-a", 3, now.Add(10*time.Second), 9)
	third.Snapshot.FaultReasonCounts[admissionobs.ReasonPostFailed] = 2
	result, err = repo.ApplyMemberReport(ctx, cluster.ID, cluster.Name, third, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.SequenceGap != 1 {
		t.Fatalf("sequence gap = %d, want 1", result.SequenceGap)
	}

	aggregates, err := repo.Aggregate(ctx, &cluster.ID, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 {
		t.Fatalf("aggregates = %d, want 1", len(aggregates))
	}
	got := aggregates[0]
	if got.Generated != 9 || got.SequenceGapCount != 1 || got.SeriesStartedCount != 1 {
		t.Fatalf("aggregate = %+v", got)
	}
	if got.FaultReasonCounts[admissionobs.ReasonPostFailed] != 2 {
		t.Fatalf("fault reasons = %+v", got.FaultReasonCounts)
	}
	var reloaded dbrepo.Cluster
	if err := db.First(&reloaded, cluster.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.ObservabilityOnboardedAt == nil {
		t.Fatal("cluster was not marked observability-onboarded")
	}
}

func TestObservabilityRepoTreatsDuplicateAsNoOp(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "duplicate-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	repo := dbrepo.NewObservabilityRepo(db)
	now := time.Now().UTC()
	report := observationReport("series-duplicate", 1, now, 5)
	if _, err := repo.ApplyMemberReport(ctx, cluster.ID, cluster.Name, report, now); err != nil {
		t.Fatal(err)
	}
	result, err := repo.ApplyMemberReport(ctx, cluster.ID, cluster.Name, report, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Duplicate {
		t.Fatalf("duplicate result = %+v", result)
	}
	aggregates, err := repo.Aggregate(ctx, &cluster.ID, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 || aggregates[0].Generated != 5 {
		t.Fatalf("aggregates = %+v", aggregates)
	}
}

func TestObservabilityRepoMissingCursorBuildsDiagnosticBaseline(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	cluster := &dbrepo.Cluster{Name: "baseline-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	repo := dbrepo.NewObservabilityRepo(db)
	now := time.Now().UTC()
	result, err := repo.ApplyMemberReport(ctx, cluster.ID, cluster.Name, observationReport("late-series", 8, now, 1000), now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.BaselineReset {
		t.Fatalf("result = %+v", result)
	}
	aggregates, err := repo.Aggregate(ctx, &cluster.ID, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(aggregates) != 1 || aggregates[0].Generated != 0 || aggregates[0].BaselineResetCount != 1 || aggregates[0].SeriesStartedCount != 0 {
		t.Fatalf("aggregates = %+v", aggregates)
	}
}

func TestObservabilityRepoSeriesIdentityIsScopedByCluster(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	clusters := dbrepo.NewClusterRepo(db)
	left := &dbrepo.Cluster{Name: "left-cluster", Phase: "Online"}
	right := &dbrepo.Cluster{Name: "right-cluster", Phase: "Online"}
	if err := clusters.Create(ctx, left); err != nil {
		t.Fatal(err)
	}
	if err := clusters.Create(ctx, right); err != nil {
		t.Fatal(err)
	}
	repo := dbrepo.NewObservabilityRepo(db)
	now := time.Now().UTC()
	for _, cluster := range []*dbrepo.Cluster{left, right} {
		if _, err := repo.ApplyMemberReport(ctx, cluster.ID, cluster.Name, observationReport("same-series", 1, now, 2), now); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&dbrepo.ObservationReportCursor{}).Where("series_id = ?", "same-series").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("cursor count = %d, want 2", count)
	}
}

func TestObservabilityRepoCleanupSeparatesBucketAndCursorRetention(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := dbrepo.NewObservabilityRepo(db)
	now := time.Now().UTC()
	oldBucket := dbrepo.ObservationBucketDelta{
		BucketStartedAt: now.Add(-31 * 24 * time.Hour),
		ClusterID:       0,
		Component:       admissionobs.ComponentCleanup,
		LastObservedAt:  now.Add(-31 * 24 * time.Hour),
	}
	if err := repo.UpsertBucket(ctx, oldBucket); err != nil {
		t.Fatal(err)
	}
	cluster := &dbrepo.Cluster{Name: "cursor-cleanup-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApplyMemberReport(ctx, cluster.ID, cluster.Name, observationReport("idle-series", 1, now.Add(-25*time.Hour), 1), now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}

	buckets, cursors, err := repo.Cleanup(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if buckets != 1 || cursors != 1 {
		t.Fatalf("cleanup = (%d, %d), want (1, 1)", buckets, cursors)
	}
}

func TestObservabilityRepoRequiresConsecutiveMinutesForSustainedPressure(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	repo := dbrepo.NewObservabilityRepo(db)
	cluster := &dbrepo.Cluster{Name: "pressure-cluster", Phase: "Online"}
	if err := dbrepo.NewClusterRepo(db).Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	write := func(bucket time.Time) {
		t.Helper()
		if err := repo.UpsertBucket(ctx, dbrepo.ObservationBucketDelta{
			BucketStartedAt: bucket,
			ClusterID:       cluster.ID,
			ClusterName:     cluster.Name,
			Component:       admissionobs.ComponentOperator,
			Snapshot: admissionobs.Snapshot{Counters: admissionobs.Counters{
				QueueRejected: 1,
			}},
			QueueDepthMax:  85,
			QueueCapacity:  100,
			LastObservedAt: bucket,
		}); err != nil {
			t.Fatal(err)
		}
	}
	write(now)
	pressure, err := repo.SustainedPressure(ctx, now, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if pressure[cluster.ID] {
		t.Fatal("one pressure minute was treated as sustained")
	}
	write(now.Add(-time.Minute))
	pressure, err = repo.SustainedPressure(ctx, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !pressure[cluster.ID] {
		t.Fatal("two consecutive pressure minutes were not detected")
	}
}
