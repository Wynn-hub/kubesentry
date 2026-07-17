package gateway

import (
	"context"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

// HeartbeatInfo is the in-memory record of a member's last heartbeat.
type HeartbeatInfo struct {
	LastSeen        time.Time
	OperatorVersion string
	Summary         *v1alpha1.PolicySummary
}

// HeartbeatTracker keeps heartbeats in memory so the hot path never
// touches etcd; StatusFlusher persists at a slow cadence.
type HeartbeatTracker struct {
	mu    sync.Mutex
	beats map[string]HeartbeatInfo
}

func NewHeartbeatTracker() *HeartbeatTracker {
	return &HeartbeatTracker{beats: map[string]HeartbeatInfo{}}
}

func (h *HeartbeatTracker) MarkAlive(cluster, version string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beats[cluster] = HeartbeatInfo{LastSeen: time.Now(), OperatorVersion: version}
}

// RecordStatus notes a member StatusReport: proof of life plus policy
// counts. OperatorVersion only changes via MarkAlive.
func (h *HeartbeatTracker) RecordStatus(cluster string, ready, invalid int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	info := h.beats[cluster]
	info.LastSeen = time.Now()
	info.Summary = &v1alpha1.PolicySummary{Ready: ready, Invalid: invalid}
	h.beats[cluster] = info
}

func (h *HeartbeatTracker) Snapshot() map[string]HeartbeatInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]HeartbeatInfo, len(h.beats))
	for k, v := range h.beats {
		out[k] = v
	}
	return out
}

// StatusFlusher periodically persists heartbeat state into Cluster.status.
// Runs as a manager Runnable on the gateway.
type StatusFlusher struct {
	client       client.Client
	hb           *HeartbeatTracker
	interval     time.Duration
	offlineAfter time.Duration
}

func NewStatusFlusher(c client.Client, hb *HeartbeatTracker, interval, offlineAfter time.Duration) *StatusFlusher {
	return &StatusFlusher{client: c, hb: hb, interval: interval, offlineAfter: offlineAfter}
}

func (f *StatusFlusher) Start(ctx context.Context) error {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// 落盘失败不致命：下个周期重试。
			_ = f.flushOnce(ctx)
		}
	}
}

func (f *StatusFlusher) flushOnce(ctx context.Context) error {
	var clusters v1alpha1.ClusterList
	if err := f.client.List(ctx, &clusters); err != nil {
		return err
	}
	snap := f.hb.Snapshot()
	now := time.Now()
	for i := range clusters.Items {
		cl := &clusters.Items[i]
		info, ok := snap[cl.Name]
		switch {
		case ok && now.Sub(info.LastSeen) <= f.offlineAfter:
			t := metav1.NewTime(info.LastSeen)
			cl.Status.Phase = v1alpha1.ClusterPhaseOnline
			cl.Status.LastHeartbeat = &t
			cl.Status.OperatorVersion = info.OperatorVersion
			if info.Summary != nil {
				s := *info.Summary
				cl.Status.PolicySummary = &s
			}
		case cl.Status.Phase == v1alpha1.ClusterPhaseOnline:
			// 有过心跳但超时（或 gateway 重启后内存丢失）→ Offline
			cl.Status.Phase = v1alpha1.ClusterPhaseOffline
		default:
			continue // Registering/Offline 且无新心跳：不写
		}
		if err := f.client.Status().Update(ctx, cl); err != nil {
			return err
		}
	}
	return nil
}
