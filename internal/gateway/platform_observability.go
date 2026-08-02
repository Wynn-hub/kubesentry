package gateway

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

type PlatformReporter struct {
	repo       *dbrepo.PlatformObservabilityRepo
	db         *gorm.DB
	collector  *platformobs.Collector
	instance   string
	isLeader   func() bool
	interval   time.Duration
	startedAt  time.Time
	cleanupErr atomic.Int32
}

// platformLeadershipGrace bounds how long the first sample waits for leader
// election. Longer than the 2s RetryPeriod so an uncontested single replica
// resolves well within it, short enough that a real standby is not silently
// missing from the overview.
const platformLeadershipGrace = 8 * time.Second

func NewPlatformReporter(
	repo *dbrepo.PlatformObservabilityRepo,
	db *gorm.DB,
	instance string,
	isLeader func() bool,
	interval time.Duration,
) *PlatformReporter {
	if interval <= 0 {
		interval = platformobs.DefaultInterval
	}
	collector := platformobs.NewCollector(platformobs.Identity{
		Component: platformobs.ComponentGateway,
		Instance:  instance,
		Role: func() platformobs.Role {
			if isLeader() {
				return platformobs.RoleLeader
			}
			return platformobs.RoleStandby
		},
	})
	return &PlatformReporter{
		repo: repo, db: db, collector: collector, instance: instance,
		isLeader: isLeader, interval: interval, startedAt: platformobs.NormalizeProcessStart(time.Now()),
	}
}

func (r *PlatformReporter) Start(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	cleanupTicker := time.NewTicker(time.Hour)
	defer cleanupTicker.Stop()
	// The reporter starts before leader election resolves, so an immediate first
	// sample would record this pod as standby and — for a single-replica install
	// where the only instance must be the leader — make the component look
	// degraded ("no leader") for its first minute. Wait for the outcome instead.
	if !r.awaitLeadershipDecision(ctx) {
		return nil
	}
	for {
		r.collect(ctx)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-cleanupTicker.C:
			if r.isLeader() {
				r.cleanup(ctx)
			}
		}
	}
}

// awaitLeadershipDecision blocks until this pod becomes leader or the grace
// period elapses, whichever comes first. A genuine standby simply waits out the
// grace period and then reports standby, which is correct.
func (r *PlatformReporter) awaitLeadershipDecision(ctx context.Context) bool {
	deadline := time.After(platformLeadershipGrace)
	poll := time.NewTicker(200 * time.Millisecond)
	defer poll.Stop()
	for {
		if r.isLeader() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			return true
		case <-poll.C:
		}
	}
}

func (r *PlatformReporter) collect(ctx context.Context) {
	if r.repo == nil {
		return
	}
	settings, err := r.repo.GetSettings(ctx)
	if err != nil {
		slog.Warn("read platform observability settings", "error", err)
		return
	}
	if !settings.Enabled {
		return
	}
	sample := r.collector.Collect()
	if r.cleanupErr.Load() >= 2 {
		sample.Health = platformobs.HealthDegraded
		sample.HealthReason = "platform_observation_cleanup_failed"
	}
	if _, err := r.repo.UpsertSample(ctx, sample, time.Now()); err != nil {
		slog.Warn("store gateway platform sample", "error", err)
	}
	if r.isLeader() {
		sample, err := r.postgresSample(ctx)
		if err != nil {
			slog.Warn("collect postgres platform sample", "error", err)
			sample.Health = platformobs.HealthDegraded
			if sample.HealthReason == "" {
				sample.HealthReason = "postgres_statistics_unavailable"
			}
		}
		if _, err := r.repo.UpsertSample(ctx, sample, time.Now()); err != nil {
			slog.Warn("store postgres platform sample", "error", err)
		}
	}
}

func (r *PlatformReporter) cleanup(ctx context.Context) {
	if _, _, err := r.repo.Cleanup(ctx, time.Now()); err != nil {
		r.cleanupErr.Add(1)
		slog.Warn("clean platform observations", "error", err)
		return
	}
	r.cleanupErr.Store(0)
}

func (r *PlatformReporter) postgresSample(ctx context.Context) (platformobs.Sample, error) {
	now := time.Now().UTC()
	sample := platformobs.UnavailableSample(platformobs.ComponentPostgres, "postgres", now)
	sample.ProcessStartedAt = r.startedAt
	sample.DBStats = map[string]any{}

	var connections, commits, rollbacks, blocksRead, blocksHit int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT numbackends, xact_commit, xact_rollback, blks_read, blks_hit
		FROM pg_stat_database WHERE datname = current_database()`).
		Row().Scan(&connections, &commits, &rollbacks, &blocksRead, &blocksHit)
	if err != nil {
		sample.Health = platformobs.HealthDegraded
		sample.HealthReason = "postgres_statistics_unavailable"
		return sample, err
	}
	var databaseBytes int64
	if err := r.db.WithContext(ctx).Raw(`SELECT pg_database_size(current_database())`).Row().Scan(&databaseBytes); err != nil {
		return sample, err
	}
	var startedAt time.Time
	if err := r.db.WithContext(ctx).Raw(`SELECT pg_postmaster_start_time()`).Row().Scan(&startedAt); err == nil {
		sample.ProcessStartedAt = platformobs.NormalizeProcessStart(startedAt)
	}
	var maxConnections int64
	if err := r.db.WithContext(ctx).Raw(`SHOW max_connections`).Row().Scan(&maxConnections); err != nil {
		return sample, err
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return sample, err
	}
	pool := sqlDB.Stats()
	totalBlocks := blocksHit + blocksRead
	cacheHitRatio := float64(0)
	if totalBlocks > 0 {
		cacheHitRatio = float64(blocksHit) / float64(totalBlocks)
	}
	sample.DBStats = map[string]any{
		"connections":     connections,
		"maxConnections":  maxConnections,
		"transactions":    commits,
		"rollbacks":       rollbacks,
		"cacheHitRatio":   cacheHitRatio,
		"databaseBytes":   databaseBytes,
		"poolInUse":       int64(pool.InUse),
		"poolIdle":        int64(pool.Idle),
		"poolWaitCount":   pool.WaitCount,
		"poolWaitSeconds": durationSeconds(pool.WaitDuration),
	}
	return sample, nil
}

func durationSeconds(value time.Duration) float64 {
	return value.Seconds()
}
