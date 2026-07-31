package gateway

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

const (
	gatewayObservationBufferCapacity  = 512
	gatewayObservationShutdownTimeout = 5 * time.Second
)

type observationBufferKey struct {
	minute    time.Time
	clusterID int64
	component admissionobs.Component
}

type GatewayObservationCollector struct {
	repo               *dbrepo.ObservabilityRepo
	flushInterval      time.Duration
	mu                 sync.Mutex
	buffer             map[observationBufferKey]dbrepo.ObservationBucketDelta
	pendingWriteFailed atomic.Int64
	enabled            atomic.Bool
}

func NewGatewayObservationCollector(repo *dbrepo.ObservabilityRepo, flushInterval time.Duration) *GatewayObservationCollector {
	if flushInterval <= 0 {
		flushInterval = 10 * time.Second
	}
	result := &GatewayObservationCollector{
		repo:          repo,
		flushInterval: flushInterval,
		buffer:        make(map[observationBufferKey]dbrepo.ObservationBucketDelta),
	}
	result.enabled.Store(true)
	return result
}

func (c *GatewayObservationCollector) SetEnabled(enabled bool) {
	if c == nil {
		return
	}
	c.enabled.Store(enabled)
	if enabled {
		return
	}
	c.mu.Lock()
	clear(c.buffer)
	c.mu.Unlock()
	c.pendingWriteFailed.Store(0)
}

func (c *GatewayObservationCollector) Record(delta dbrepo.ObservationBucketDelta) {
	if c == nil || !c.enabled.Load() {
		return
	}
	if delta.BucketStartedAt.IsZero() {
		delta.BucketStartedAt = time.Now().UTC()
	}
	if delta.LastObservedAt.IsZero() {
		delta.LastObservedAt = time.Now().UTC()
	}
	delta.BucketStartedAt = delta.BucketStartedAt.UTC().Truncate(time.Minute)
	key := observationBufferKey{minute: delta.BucketStartedAt, clusterID: delta.ClusterID, component: delta.Component}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabled.Load() {
		return
	}
	if existing, ok := c.buffer[key]; ok {
		c.buffer[key] = mergeObservationDeltas(existing, delta)
		return
	}
	if len(c.buffer) >= gatewayObservationBufferCapacity {
		oldest := oldestObservationKey(c.buffer)
		delete(c.buffer, oldest)
		c.pendingWriteFailed.Add(1)
	}
	c.buffer[key] = delta
}

func mergeObservationDeltas(left, right dbrepo.ObservationBucketDelta) dbrepo.ObservationBucketDelta {
	left.Snapshot.Counters = left.Snapshot.Counters.Add(right.Snapshot.Counters)
	left.Snapshot.FaultReasonCounts = addReasonCounts(left.Snapshot.FaultReasonCounts, right.Snapshot.FaultReasonCounts)
	left.Snapshot.PressureReasonCounts = addReasonCounts(left.Snapshot.PressureReasonCounts, right.Snapshot.PressureReasonCounts)
	left.Snapshot.FilterReasonCounts = addReasonCounts(left.Snapshot.FilterReasonCounts, right.Snapshot.FilterReasonCounts)
	left.ObservationWriteFailed += right.ObservationWriteFailed
	left.SequenceGapCount += right.SequenceGapCount
	left.BaselineResetCount += right.BaselineResetCount
	left.SeriesStartedCount += right.SeriesStartedCount
	if right.QueueDepthMax > left.QueueDepthMax {
		left.QueueDepthMax = right.QueueDepthMax
	}
	if right.QueueCapacity > left.QueueCapacity {
		left.QueueCapacity = right.QueueCapacity
	}
	if right.LastObservedAt.After(left.LastObservedAt) {
		left.LastObservedAt = right.LastObservedAt
	}
	if right.ClusterName != "" {
		left.ClusterName = right.ClusterName
	}
	return left
}

func addReasonCounts(left, right map[string]int64) map[string]int64 {
	if left == nil {
		left = make(map[string]int64, len(right))
	}
	for reason, count := range right {
		left[reason] += count
	}
	return left
}

func oldestObservationKey(buffer map[observationBufferKey]dbrepo.ObservationBucketDelta) observationBufferKey {
	var oldest observationBufferKey
	first := true
	for key := range buffer {
		if first || key.minute.Before(oldest.minute) {
			oldest = key
			first = false
		}
	}
	return oldest
}

func (c *GatewayObservationCollector) Start(ctx context.Context) error {
	if c == nil || c.repo == nil {
		return nil
	}
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), gatewayObservationShutdownTimeout)
			c.flush(shutdownCtx)
			cancel()
			return nil
		case <-ticker.C:
			c.flush(ctx)
		}
	}
}

func (c *GatewayObservationCollector) flush(ctx context.Context) {
	if c == nil || c.repo == nil || !c.enabled.Load() {
		return
	}
	if pending := c.pendingWriteFailed.Swap(0); pending > 0 {
		if !c.enabled.Load() {
			return
		}
		now := time.Now().UTC()
		err := c.repo.UpsertBucket(ctx, dbrepo.ObservationBucketDelta{
			BucketStartedAt:        now,
			Component:              admissionobs.ComponentCleanup,
			ObservationWriteFailed: pending,
			LastObservedAt:         now,
		})
		if err != nil {
			c.pendingWriteFailed.Add(pending)
			return
		}
	}

	c.mu.Lock()
	keys := make([]observationBufferKey, 0, len(c.buffer))
	for key := range c.buffer {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].minute.Equal(keys[j].minute) {
			if keys[i].clusterID == keys[j].clusterID {
				return keys[i].component < keys[j].component
			}
			return keys[i].clusterID < keys[j].clusterID
		}
		return keys[i].minute.Before(keys[j].minute)
	})
	c.mu.Unlock()

	for _, key := range keys {
		c.mu.Lock()
		delta, ok := c.buffer[key]
		if ok {
			delete(c.buffer, key)
		}
		c.mu.Unlock()
		if !ok {
			continue
		}
		if !c.enabled.Load() {
			return
		}
		if err := c.repo.UpsertBucket(ctx, delta); err != nil {
			c.mu.Lock()
			if current, exists := c.buffer[key]; exists {
				c.buffer[key] = mergeObservationDeltas(delta, current)
			} else if len(c.buffer) < gatewayObservationBufferCapacity {
				c.buffer[key] = delta
			} else {
				c.pendingWriteFailed.Add(1)
			}
			c.mu.Unlock()
			slog.Error("flush gateway observation bucket", "cluster_id", delta.ClusterID, "component", delta.Component, "error", err)
			return
		}
	}
}

func (c *GatewayObservationCollector) PendingWriteFailures() int64 {
	if c == nil {
		return 0
	}
	return c.pendingWriteFailed.Load()
}

func (c *GatewayObservationCollector) BufferedBuckets() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.buffer)
}

func (s *Server) recordGatewayObservation(cluster *dbrepo.Cluster, snapshot admissionobs.Snapshot) {
	if s == nil || s.observationCollector == nil || !s.admissionObservationEnabled.Load() {
		return
	}
	now := time.Now().UTC()
	delta := dbrepo.ObservationBucketDelta{
		BucketStartedAt: now,
		Component:       admissionobs.ComponentGateway,
		Snapshot:        snapshot,
		LastObservedAt:  now,
	}
	if cluster != nil {
		delta.ClusterID = cluster.ID
		delta.ClusterName = cluster.Name
	}
	s.observationCollector.Record(delta)
}
