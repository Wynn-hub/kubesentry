package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type IncidentCleaner struct {
	repo     *dbrepo.IncidentRepo
	settings *dbrepo.AdmissionRepo
	interval time.Duration
	trigger  chan struct{}
	onResult func(error)
}

func NewIncidentCleaner(repo *dbrepo.IncidentRepo, settings *dbrepo.AdmissionRepo, interval time.Duration, onResult func(error)) *IncidentCleaner {
	if interval <= 0 {
		interval = time.Hour
	}
	return &IncidentCleaner{repo: repo, settings: settings, interval: interval, trigger: make(chan struct{}, 1), onResult: onResult}
}

func (c *IncidentCleaner) Trigger() {
	if c == nil || c.trigger == nil {
		return
	}
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

func (c *IncidentCleaner) Start(ctx context.Context) error {
	if c == nil || c.repo == nil || c.settings == nil {
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

func (c *IncidentCleaner) run(ctx context.Context) {
	settings, err := c.settings.GetSettings(ctx)
	if err == nil {
		var deleted int64
		deleted, err = c.repo.Cleanup(ctx, time.Now().UTC(), settings.IncidentRetentionDays)
		if err == nil && deleted > 0 {
			slog.Info("cleaned admission link incidents", "incidents", deleted)
		}
	}
	if err != nil {
		slog.Error("cleanup admission link incidents", "error", err)
	}
	if c.onResult != nil {
		c.onResult(err)
	}
}
