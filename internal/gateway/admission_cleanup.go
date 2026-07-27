package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

type AdmissionCleaner struct {
	repo     *dbrepo.AdmissionRepo
	interval time.Duration
}

func NewAdmissionCleaner(repo *dbrepo.AdmissionRepo, interval time.Duration) *AdmissionCleaner {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &AdmissionCleaner{repo: repo, interval: interval}
}
func (c *AdmissionCleaner) Start(ctx context.Context) error {
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
		}
	}
}
func (c *AdmissionCleaner) run(ctx context.Context) {
	deleted, err := c.repo.Cleanup(ctx, time.Now().UTC())
	if err != nil {
		slog.Error("cleanup admission events", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("cleaned up admission events", "deleted", deleted)
	}
}
