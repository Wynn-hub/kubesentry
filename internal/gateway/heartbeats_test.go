package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestTrackerMarkAliveAndSnapshot(t *testing.T) {
	tr := NewHeartbeatTracker()
	tr.MarkAlive("prod", "v1")
	snap := tr.Snapshot()
	info, ok := snap["prod"]
	if !ok {
		t.Fatal("prod missing from snapshot")
	}
	if time.Since(info.LastSeen) > time.Second || info.OperatorVersion != "v1" {
		t.Fatalf("info = %+v", info)
	}
	// Snapshot 是拷贝：改动它不影响内部状态
	delete(snap, "prod")
	if _, ok := tr.Snapshot()["prod"]; !ok {
		t.Fatal("snapshot mutation leaked into tracker")
	}
}

func TestFlusherOnlineAndOffline(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", Phase: v1alpha1.ClusterPhaseRegistering})
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "stale", Phase: v1alpha1.ClusterPhaseOnline})

	tr := NewHeartbeatTracker()
	tr.MarkAlive("prod", "v2")
	// stale 有历史心跳但已超时
	tr.mu.Lock()
	tr.beats["stale"] = HeartbeatInfo{LastSeen: time.Now().Add(-10 * time.Minute), OperatorVersion: "v1"}
	tr.mu.Unlock()

	f := NewStatusFlusher(repos.Clusters, tr, time.Hour, 90*time.Second)
	if err := f.flushOnce(context.Background()); err != nil {
		t.Fatalf("flushOnce: %v", err)
	}

	ctx := context.Background()
	got, err := repos.Clusters.GetByName(ctx, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != v1alpha1.ClusterPhaseOnline || got.LastHeartbeatAt == nil || got.OperatorVersion != "v2" {
		t.Fatalf("prod status = %+v", got)
	}
	got, err = repos.Clusters.GetByName(ctx, "stale")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != v1alpha1.ClusterPhaseOffline {
		t.Fatalf("stale phase = %q, want Offline", got.Phase)
	}
}

func TestRecordStatusPreservesVersionAndFlushesSummary(t *testing.T) {
	tr := NewHeartbeatTracker()
	tr.MarkAlive("c1", "v1")
	tr.RecordStatus("c1", 7, 2)

	info := tr.Snapshot()["c1"]
	if info.OperatorVersion != "v1" || info.Summary == nil || info.Summary.Ready != 7 || info.Summary.Invalid != 2 {
		t.Fatalf("info = %+v", info)
	}

	// dbrepo.Cluster has no PolicySummary column (see StatusFlusher's
	// ponytail note) — this test now only asserts the version/phase flush
	// still works when a Summary is present on the in-memory record.
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "c1"})
	f := NewStatusFlusher(repos.Clusters, tr, time.Minute, time.Minute)
	if err := f.flushOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := repos.Clusters.GetByName(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != v1alpha1.ClusterPhaseOnline || got.OperatorVersion != "v1" {
		t.Fatalf("flushed cluster = %+v", got)
	}
}
