package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

const PlatformObservabilitySettingsTopic = "platform-observability-settings"

type ObservabilityCleaner struct {
	repo      *dbrepo.ObservabilityRepo
	collector *GatewayObservationCollector
	interval  time.Duration
	trigger   chan struct{}
}

func NewObservabilityCleaner(repo *dbrepo.ObservabilityRepo, collector *GatewayObservationCollector, interval time.Duration) *ObservabilityCleaner {
	if interval <= 0 {
		interval = time.Hour
	}
	return &ObservabilityCleaner{repo: repo, collector: collector, interval: interval, trigger: make(chan struct{}, 1)}
}

func (c *ObservabilityCleaner) Trigger() {
	if c == nil || c.trigger == nil {
		return
	}
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

func (c *ObservabilityCleaner) Start(ctx context.Context) error {
	if c == nil || c.repo == nil {
		return nil
	}
	c.run(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.run(ctx)
		case <-c.trigger:
			c.run(ctx)
		}
	}
}

func (c *ObservabilityCleaner) run(ctx context.Context) {
	now := time.Now().UTC()
	counters := admissionobs.Counters{CleanupRun: 1}
	buckets, cursors, err := c.repo.Cleanup(ctx, now)
	if err != nil {
		counters.CleanupFailure = 1
		slog.Error("cleanup platform observations", "error", err)
	} else {
		counters.CleanupDeleted = buckets + cursors
		if counters.CleanupDeleted > 0 {
			slog.Info("cleaned platform observations", "buckets", buckets, "cursors", cursors)
		}
	}
	c.collector.Record(dbrepo.ObservationBucketDelta{
		BucketStartedAt: now,
		Component:       admissionobs.ComponentCleanup,
		Snapshot:        admissionobs.Snapshot{Counters: counters},
		LastObservedAt:  now,
	})
}
