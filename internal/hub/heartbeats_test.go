package hub

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
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
	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "prod"}}
	cl.Status.Phase = v1alpha1.ClusterPhaseRegistering
	stale := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "stale"}}
	stale.Status.Phase = v1alpha1.ClusterPhaseOnline
	c := newHubClient(t, cl, stale)

	tr := NewHeartbeatTracker()
	tr.MarkAlive("prod", "v2")
	// stale 有历史心跳但已超时
	tr.mu.Lock()
	tr.beats["stale"] = HeartbeatInfo{LastSeen: time.Now().Add(-10 * time.Minute), OperatorVersion: "v1"}
	tr.mu.Unlock()

	f := NewStatusFlusher(c, tr, time.Hour, 90*time.Second)
	if err := f.flushOnce(context.Background()); err != nil {
		t.Fatalf("flushOnce: %v", err)
	}

	ctx := context.Background()
	var got v1alpha1.Cluster
	_ = c.Get(ctx, types.NamespacedName{Name: "prod"}, &got)
	if got.Status.Phase != v1alpha1.ClusterPhaseOnline || got.Status.LastHeartbeat == nil ||
		got.Status.OperatorVersion != "v2" {
		t.Fatalf("prod status = %+v", got.Status)
	}
	_ = c.Get(ctx, types.NamespacedName{Name: "stale"}, &got)
	if got.Status.Phase != v1alpha1.ClusterPhaseOffline {
		t.Fatalf("stale phase = %q, want Offline", got.Status.Phase)
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

	cl := &v1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	c := newHubClient(t, cl)
	f := NewStatusFlusher(c, tr, time.Minute, time.Minute)
	if err := f.flushOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got v1alpha1.Cluster
	if err := c.Get(context.Background(), types.NamespacedName{Name: "c1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.PolicySummary == nil || got.Status.PolicySummary.Ready != 7 {
		t.Fatalf("summary not flushed: %+v", got.Status)
	}
}
