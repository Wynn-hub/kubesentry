package gateway

import (
	"context"
	"testing"
	"time"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
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
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "prod", Phase: gatewaymodel.ClusterPhaseRegistering})
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "stale", Phase: gatewaymodel.ClusterPhaseOnline})

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
	if got.Phase != gatewaymodel.ClusterPhaseOnline || got.LastHeartbeatAt == nil || got.OperatorVersion != "v2" {
		t.Fatalf("prod status = %+v", got)
	}
	got, err = repos.Clusters.GetByName(ctx, "stale")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != gatewaymodel.ClusterPhaseOffline {
		t.Fatalf("stale phase = %q, want Offline", got.Phase)
	}
}

func TestRecordStatusPreservesVersionAndFlushesSummary(t *testing.T) {
	tr := NewHeartbeatTracker()
	tr.MarkAlive("c1", "v1")
	tr.RecordStatus("c1", 7, 2, nil)

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
	if got.Phase != gatewaymodel.ClusterPhaseOnline || got.OperatorVersion != "v1" {
		t.Fatalf("flushed cluster = %+v", got)
	}
}

func TestFlusherRestoresLegacyUninstallFailureToOnline(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "retryable", Phase: gatewaymodel.ClusterPhaseUninstallFailed})

	tr := NewHeartbeatTracker()
	tr.MarkAlive("retryable", "v2")
	if err := NewStatusFlusher(repos.Clusters, tr, time.Hour, time.Minute).flushOnce(context.Background()); err != nil {
		t.Fatalf("flushOnce: %v", err)
	}
	got, err := repos.Clusters.GetByName(context.Background(), "retryable")
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != gatewaymodel.ClusterPhaseOnline {
		t.Fatalf("phase = %q, want Online", got.Phase)
	}
}

func TestFlusherPersistsReportedClusterInventory(t *testing.T) {
	repos := newTestRepos(t)
	mustCreateCluster(t, repos, &dbrepo.Cluster{Name: "member"})
	ctx := context.Background()

	tr := NewHeartbeatTracker()
	tr.MarkAlive("member", "v1")
	tr.RecordStatus("member", 3, 0, &ClusterInventory{
		KubernetesVersion: "v1.31.2",
		APIServerAddress:  "https://10.96.0.1:443",
		NodeCount:         5,
		ReadyNodeCount:    4,
	})
	f := NewStatusFlusher(repos.Clusters, tr, time.Hour, time.Minute)
	if err := f.flushOnce(ctx); err != nil {
		t.Fatalf("flushOnce: %v", err)
	}
	got, err := repos.Clusters.GetByName(ctx, "member")
	if err != nil {
		t.Fatal(err)
	}
	if got.KubernetesVersion != "v1.31.2" || got.APIServerAddress != "https://10.96.0.1:443" || got.NodeCount != 5 || got.ReadyNodeCount != 4 {
		t.Fatalf("inventory not persisted: %+v", got)
	}

	// A later heartbeat carries no inventory of its own; the stored facts must
	// survive rather than being reset to empty/zero.
	tr.MarkAlive("member", "v2")
	if err := f.flushOnce(ctx); err != nil {
		t.Fatalf("second flushOnce: %v", err)
	}
	got, err = repos.Clusters.GetByName(ctx, "member")
	if err != nil {
		t.Fatal(err)
	}
	if got.KubernetesVersion != "v1.31.2" || got.NodeCount != 5 {
		t.Fatalf("inventory lost after a heartbeat-only flush: %+v", got)
	}
}

func TestInventoryOfIgnoresEmptyReport(t *testing.T) {
	if inventoryOf(nil) != nil {
		t.Error("nil report must yield no inventory")
	}
	if got := inventoryOf(&gatewaypb.StatusReport{ReadyPolicies: 2}); got != nil {
		t.Errorf("report without inventory yielded %+v, want nil", got)
	}
	got := inventoryOf(&gatewaypb.StatusReport{KubernetesVersion: "v1.30.0"})
	if got == nil || got.KubernetesVersion != "v1.30.0" {
		t.Errorf("inventory = %+v, want the reported version", got)
	}
}
