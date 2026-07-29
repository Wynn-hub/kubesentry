package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// AdmissionEventSettingsTopic is the NotifyChange topic console sends after
// writing the admission event configuration. The gateway treats it as the
// trigger for a full cleanup pass, so a narrowed rule set takes effect within
// seconds instead of waiting out the daily ticker.
const AdmissionEventSettingsTopic = "admission-event-settings"

type AdmissionCleaner struct {
	repo     *dbrepo.AdmissionRepo
	interval time.Duration
	// full is the configuration-changed trigger. Buffered with capacity 1 so
	// TriggerFullCleanup never blocks its caller (a gRPC handler) and a burst
	// of saves coalesces into one pass — the pass always reads the latest
	// configuration from the database anyway.
	full chan struct{}
}

func NewAdmissionCleaner(repo *dbrepo.AdmissionRepo, interval time.Duration) *AdmissionCleaner {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &AdmissionCleaner{repo: repo, interval: interval, full: make(chan struct{}, 1)}
}

// TriggerFullCleanup asks for a full pass without waiting for it. Safe on a nil
// cleaner and on one whose Start was never called: the trigger is dropped
// rather than queued forever.
func (c *AdmissionCleaner) TriggerFullCleanup() {
	if c == nil || c.full == nil {
		return
	}
	select {
	case c.full <- struct{}{}:
	default:
	}
}

func (c *AdmissionCleaner) Start(ctx context.Context) error {
	if c == nil || c.repo == nil {
		return nil
	}
	c.run(ctx, false)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.run(ctx, false)
		case <-c.full:
			c.run(ctx, true)
		}
	}
}

// run performs one pass. The routine (ticker) pass skips events younger than
// the shortest allowed retention, since no configuration can expire them yet.
// A configuration change needs the full range: narrowing the rule set makes
// matching events ineligible at any age.
func (c *AdmissionCleaner) run(ctx context.Context, full bool) {
	now := time.Now().UTC()
	cleanup := c.repo.Cleanup
	if full {
		cleanup = c.repo.CleanupAll
	}
	deleted, err := cleanup(ctx, now)
	if err != nil {
		slog.Error("cleanup admission events", "full", full, "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("cleaned up admission events", "full", full, "deleted", deleted)
	}
}
