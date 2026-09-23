package gateway

import (
	"context"
	"sync"
	"time"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// HeartbeatInfo is the in-memory record of a member's last heartbeat.
type HeartbeatInfo struct {
	LastSeen        time.Time
	OperatorVersion string
	Summary         *gatewaymodel.PolicySummary
	// Inventory is the member's self-reported cluster facts from the last
	// StatusReport, nil until one arrives. Heartbeats alone never set it.
	Inventory *ClusterInventory
}

// ClusterInventory is the member-reported cluster description carried by
// StatusReport. Fields the member could not read stay empty/zero and are not
// persisted, so a transient read failure never wipes a known value.
type ClusterInventory struct {
	KubernetesVersion string
	APIServerAddress  string
	NodeCount         int
	ReadyNodeCount    int
}

// inventoryOf extracts the cluster inventory from a StatusReport, returning nil
// when the member reported nothing (an older operator, or one that could read
// none of the values) so the tracker keeps any previously reported facts.
func inventoryOf(rep *gatewaypb.StatusReport) *ClusterInventory {
	if rep == nil {
		return nil
	}
	if rep.KubernetesVersion == "" && rep.ApiServerAddress == "" && rep.NodeCount == 0 {
		return nil
	}
	return &ClusterInventory{
		KubernetesVersion: rep.KubernetesVersion,
		APIServerAddress:  rep.ApiServerAddress,
		NodeCount:         int(rep.NodeCount),
		ReadyNodeCount:    int(rep.ReadyNodeCount),
	}
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
	// Preserve the inventory a previous StatusReport supplied: heartbeats are
	// far more frequent and carry no inventory of their own.
	info := h.beats[cluster]
	info.LastSeen = time.Now()
	info.OperatorVersion = version
	info.Summary = nil
	h.beats[cluster] = info
}

// RecordStatus notes a member StatusReport: proof of life, policy counts and
// the reported cluster inventory. OperatorVersion only changes via MarkAlive.
func (h *HeartbeatTracker) RecordStatus(cluster string, ready, invalid int32, inventory *ClusterInventory) {
	h.mu.Lock()
	defer h.mu.Unlock()
	info := h.beats[cluster]
	info.LastSeen = time.Now()
	info.Summary = &gatewaymodel.PolicySummary{Ready: ready, Invalid: invalid}
	if inventory != nil {
		info.Inventory = inventory
	}
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

// StatusFlusher periodically persists heartbeat state into dbrepo.Cluster
// (Phase/OperatorVersion/LastHeartbeatAt). Runs as a background goroutine on
// the gateway, alongside the gRPC and health servers.
//
// ponytail: dbrepo.Cluster has no PolicySummary column (unlike the old
// gatewaymodel.ClusterStatus field) — ready/invalid policy counts are
// tracked in-memory only via HeartbeatTracker.Snapshot()[x].Summary for now.
// Add a column + persist here if a DB-backed summary view turns out to be
// needed (e.g. for the console-api cluster list).
type StatusFlusher struct {
	clusters     *dbrepo.ClusterRepo
	hb           *HeartbeatTracker
	interval     time.Duration
	offlineAfter time.Duration
}

func NewStatusFlusher(clusters *dbrepo.ClusterRepo, hb *HeartbeatTracker, interval, offlineAfter time.Duration) *StatusFlusher {
	return &StatusFlusher{clusters: clusters, hb: hb, interval: interval, offlineAfter: offlineAfter}
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
	all, err := f.clusters.List(ctx)
	if err != nil {
		return err
	}
	snap := f.hb.Snapshot()
	now := time.Now()
	for i := range all {
		cl := &all[i]
		// An uninstall task owns the active and completed lifecycle states. A
		// delayed heartbeat must never resurrect a cluster after it has started
		// or completed its self-removal. Failed uninstalls deliberately remain
		// eligible for normal heartbeat reconciliation: their operator is still
		// connected and the cluster must return to Online.
		if cl.Phase == gatewaymodel.ClusterPhaseUninstalling ||
			cl.Phase == gatewaymodel.ClusterPhaseUninstalled {
			continue
		}
		info, ok := snap[cl.Name]
		switch {
		case ok && now.Sub(info.LastSeen) <= f.offlineAfter:
			t := info.LastSeen
			cl.Phase = gatewaymodel.ClusterPhaseOnline
			cl.LastHeartbeatAt = &t
			cl.OperatorVersion = info.OperatorVersion
		case cl.Phase == gatewaymodel.ClusterPhaseOnline:
			// 有过心跳但超时（或 gateway 重启后内存丢失）→ Offline
			cl.Phase = gatewaymodel.ClusterPhaseOffline
		default:
			continue // Registering/Offline 且无新心跳：不写
		}
		if err := f.clusters.UpdateHeartbeat(ctx, cl.ID, cl.Phase, cl.LastHeartbeatAt, cl.OperatorVersion); err != nil {
			return err
		}
		// Inventory is written separately and only when reported, so a member
		// that never reports it (or failed to read it) keeps whatever the
		// database already has instead of being reset to empty/zero.
		if ok && info.Inventory != nil {
			inv := *info.Inventory
			if err := f.clusters.UpdateInventory(ctx, cl.ID, dbrepo.ClusterInventory{
				KubernetesVersion: inv.KubernetesVersion,
				APIServerAddress:  inv.APIServerAddress,
				NodeCount:         inv.NodeCount,
				ReadyNodeCount:    inv.ReadyNodeCount,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
