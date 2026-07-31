package dbrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
	"github.com/Wynn-hub/kubesentry/internal/platformobs"
)

const (
	PlatformSampleRetention     = 7 * 24 * time.Hour
	PlatformLeadershipRetention = 30 * 24 * time.Hour
	platformCleanupBatchSize    = 1000
)

type PlatformObservabilityRepo struct {
	db *gorm.DB
}

func NewPlatformObservabilityRepo(db *gorm.DB) *PlatformObservabilityRepo {
	return &PlatformObservabilityRepo{db: db}
}

type PlatformSampleResult struct {
	RestartCount int64
	RoleChanged  bool
}

func (r *PlatformObservabilityRepo) UpsertSample(
	ctx context.Context,
	sample platformobs.Sample,
	receivedAt time.Time,
) (PlatformSampleResult, error) {
	if err := platformobs.Validate(sample); err != nil {
		return PlatformSampleResult{}, err
	}
	if sample.ClusterID < 0 {
		return PlatformSampleResult{}, errors.New("cluster id must not be negative")
	}
	receivedAt = receivedAt.UTC()
	bucketStartedAt := observationBucketTime(sample.ObservedAt, receivedAt)
	result := PlatformSampleResult{}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous PlatformComponentSample
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("component = ? AND instance = ? AND cluster_id = ?",
				sample.Component, sample.Instance, sample.ClusterID).
			Order("last_observed_at DESC").
			Take(&previous).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			err = nil
		case err != nil:
			return fmt.Errorf("query previous platform sample: %w", err)
		default:
			result.RestartCount = previous.RestartCount
			if !previous.ProcessStartedAt.Equal(sample.ProcessStartedAt.UTC()) {
				result.RestartCount++
			}
			if leadershipComponent(sample.Component) && previous.Role != string(sample.Role) {
				result.RoleChanged = true
				event := PlatformLeadershipEvent{
					ID:          dbmodel.NewID(),
					Component:   string(sample.Component),
					ClusterID:   sample.ClusterID,
					ClusterName: sample.ClusterName,
					Instance:    sample.Instance,
					RoleFrom:    previous.Role,
					RoleTo:      string(sample.Role),
					ChangedAt:   receivedAt,
					DetectedBy:  "sample",
					CreatedAt:   receivedAt,
				}
				if err := tx.Create(&event).Error; err != nil {
					return fmt.Errorf("create platform leadership event: %w", err)
				}
			}
		}

		row := platformSampleRow(sample, bucketStartedAt, receivedAt, result.RestartCount)
		err = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "bucket_started_at"},
				{Name: "component"},
				{Name: "instance"},
				{Name: "cluster_id"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"cluster_name":          row.ClusterName,
				"role":                  row.Role,
				"process_started_at":    row.ProcessStartedAt,
				"restart_count":         row.RestartCount,
				"memory_bytes":          row.MemoryBytes,
				"memory_limit_bytes":    row.MemoryLimitBytes,
				"cpu_usage_micros":      row.CPUUsageMicros,
				"cpu_quota_micros":      row.CPUQuotaMicros,
				"goroutines":            row.Goroutines,
				"heap_alloc_bytes":      row.HeapAllocBytes,
				"heap_sys_bytes":        row.HeapSysBytes,
				"gc_pause_total_micros": row.GCPauseTotalMicros,
				"gc_count":              row.GCCount,
				"health":                row.Health,
				"health_reason":         row.HealthReason,
				"db_stats":              row.DBStats,
				"last_observed_at":      row.LastObservedAt,
				"updated_at":            receivedAt,
			}),
		}).Create(&row).Error
		if err != nil {
			return fmt.Errorf("upsert platform component sample: %w", err)
		}
		return nil
	})
	return result, err
}

func platformSampleRow(sample platformobs.Sample, bucket, receivedAt time.Time, restartCount int64) PlatformComponentSample {
	stats := sample.DBStats
	if stats == nil {
		stats = map[string]any{}
	}
	return PlatformComponentSample{
		ID:                 dbmodel.NewID(),
		BucketStartedAt:    bucket,
		Component:          string(sample.Component),
		Instance:           sample.Instance,
		ClusterID:          sample.ClusterID,
		ClusterName:        sample.ClusterName,
		Role:               string(sample.Role),
		ProcessStartedAt:   sample.ProcessStartedAt.UTC(),
		RestartCount:       restartCount,
		MemoryBytes:        sample.MemoryBytes,
		MemoryLimitBytes:   sample.MemoryLimitBytes,
		CPUUsageMicros:     sample.CPUUsageMicros,
		CPUQuotaMicros:     sample.CPUQuotaMicros,
		Goroutines:         sample.Goroutines,
		HeapAllocBytes:     sample.HeapAllocBytes,
		HeapSysBytes:       sample.HeapSysBytes,
		GCPauseTotalMicros: sample.GCPauseTotalMicros,
		GCCount:            sample.GCCount,
		Health:             string(sample.Health),
		HealthReason:       sample.HealthReason,
		DBStats:            datatypes.NewJSONType(stats),
		LastObservedAt:     receivedAt,
		CreatedAt:          receivedAt,
		UpdatedAt:          receivedAt,
	}
}

func leadershipComponent(component platformobs.Component) bool {
	return component == platformobs.ComponentGateway || component == platformobs.ComponentOperator
}

func (r *PlatformObservabilityRepo) RecordSessionLost(
	ctx context.Context,
	component platformobs.Component,
	clusterID int64,
	clusterName, instance string,
	changedAt time.Time,
) error {
	if !leadershipComponent(component) {
		return errors.New("session loss is only valid for leadership components")
	}
	event := PlatformLeadershipEvent{
		ID:          dbmodel.NewID(),
		Component:   string(component),
		ClusterID:   clusterID,
		ClusterName: clusterName,
		Instance:    instance,
		RoleFrom:    string(platformobs.RoleLeader),
		RoleTo:      string(platformobs.RoleStandby),
		ChangedAt:   changedAt.UTC(),
		DetectedBy:  "session_lost",
		CreatedAt:   changedAt.UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&event).Error; err != nil {
		return fmt.Errorf("record lost leader session: %w", err)
	}
	return nil
}

func (r *PlatformObservabilityRepo) GetSettings(ctx context.Context) (*PlatformObservabilitySettings, error) {
	var settings PlatformObservabilitySettings
	if err := r.db.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return nil, fmt.Errorf("get platform observability settings: %w", err)
	}
	return &settings, nil
}

func (r *PlatformObservabilityRepo) SetAdmissionPipelineEnabled(ctx context.Context, enabled bool) error {
	result := r.db.WithContext(ctx).Model(&PlatformObservabilitySettings{}).
		Where("id = 1").
		Updates(map[string]any{
			"admission_pipeline_enabled": enabled,
			"updated_at":                 time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update platform observability settings: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("platform observability settings row not found")
	}
	return nil
}

func (r *PlatformObservabilityRepo) LatestSamples(ctx context.Context, since time.Time) ([]PlatformComponentSample, error) {
	var rows []PlatformComponentSample
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (component, instance, cluster_id) *
		FROM kubesentry.platform_component_samples
		WHERE last_observed_at >= ?
		ORDER BY component, instance, cluster_id, last_observed_at DESC`,
		since.UTC()).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query latest platform samples: %w", err)
	}
	return rows, nil
}

type PlatformTrendRow struct {
	BucketStartedAt time.Time `json:"bucketStartedAt"`
	MemoryBytes     int64     `json:"memoryBytes"`
	CPUUsageMicros  int64     `json:"cpuUsageMicros"`
	Goroutines      int64     `json:"goroutines"`
	GCPauseMicros   int64     `json:"gcPauseMicros"`
}

type PlatformRestartedInstance struct {
	Component string
	Instance  string
	ClusterID int64
}

type PlatformCPUUsage struct {
	Component      string
	Instance       string
	ClusterID      int64
	CPUUsageMicros int64
}

func (r *PlatformObservabilityRepo) LatestCPUUsage(
	ctx context.Context,
	since time.Time,
) ([]PlatformCPUUsage, error) {
	var rows []PlatformCPUUsage
	err := r.db.WithContext(ctx).Raw(`
		WITH ranked AS (
			SELECT component, instance, cluster_id, cpu_usage_micros,
			       ROW_NUMBER() OVER (
			           PARTITION BY component, instance, cluster_id
			           ORDER BY bucket_started_at DESC
			       ) AS row_number
			FROM kubesentry.platform_component_samples
			WHERE bucket_started_at >= ? AND cpu_usage_micros >= 0
		)
		SELECT latest.component, latest.instance, latest.cluster_id,
		       CASE
		           WHEN previous.cpu_usage_micros IS NULL THEN -1
		           ELSE GREATEST(0, latest.cpu_usage_micros - previous.cpu_usage_micros)
		       END AS cpu_usage_micros
		FROM ranked latest
		LEFT JOIN ranked previous
		  ON previous.component = latest.component
		 AND previous.instance = latest.instance
		 AND previous.cluster_id = latest.cluster_id
		 AND previous.row_number = 2
		WHERE latest.row_number = 1`,
		since.UTC()).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query latest platform cpu usage: %w", err)
	}
	return rows, nil
}

func (r *PlatformObservabilityRepo) RestartedInstances(
	ctx context.Context,
	since time.Time,
) ([]PlatformRestartedInstance, error) {
	var rows []PlatformRestartedInstance
	err := r.db.WithContext(ctx).Raw(`
		WITH current_window AS (
			SELECT component, instance, cluster_id,
			       MIN(restart_count) AS minimum_restart_count,
			       MAX(restart_count) AS maximum_restart_count
			FROM kubesentry.platform_component_samples
			WHERE last_observed_at >= ?
			GROUP BY component, instance, cluster_id
		), baseline AS (
			SELECT DISTINCT ON (component, instance, cluster_id)
			       component, instance, cluster_id, restart_count
			FROM kubesentry.platform_component_samples
			WHERE last_observed_at < ?
			ORDER BY component, instance, cluster_id, last_observed_at DESC
		)
		SELECT current_window.component, current_window.instance, current_window.cluster_id
		FROM current_window
		LEFT JOIN baseline USING (component, instance, cluster_id)
		WHERE current_window.maximum_restart_count >
		      COALESCE(baseline.restart_count, current_window.minimum_restart_count)`,
		since.UTC(), since.UTC()).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query restarted platform instances: %w", err)
	}
	return rows, nil
}

func (r *PlatformObservabilityRepo) Trend(
	ctx context.Context,
	component, instance string,
	clusterID int64,
	from, to time.Time,
	step time.Duration,
) ([]PlatformTrendRow, error) {
	if step < time.Minute || step%time.Minute != 0 {
		return nil, errors.New("platform trend step must be whole minutes")
	}
	stepSeconds := int64(step / time.Second)
	var rows []PlatformTrendRow
	err := r.db.WithContext(ctx).Raw(`
		WITH samples AS (
			SELECT
				to_timestamp(FLOOR(EXTRACT(EPOCH FROM bucket_started_at) / ?) * ?) AS bucket_started_at,
				memory_bytes,
				cpu_usage_micros,
				goroutines,
				gc_pause_total_micros
			FROM kubesentry.platform_component_samples
			WHERE component = ? AND instance = ? AND cluster_id = ?
			  AND bucket_started_at >= ? AND bucket_started_at < ?
		), grouped AS (
			SELECT bucket_started_at,
			       MAX(memory_bytes) AS memory_bytes,
			       MAX(cpu_usage_micros) AS cpu_usage_micros,
			       MAX(goroutines) AS goroutines,
			       MAX(gc_pause_total_micros) AS gc_pause_total_micros
			FROM samples
			GROUP BY bucket_started_at
		), rates AS (
			SELECT bucket_started_at, memory_bytes, goroutines,
			       COALESCE(GREATEST(0, cpu_usage_micros - LAG(cpu_usage_micros) OVER (ORDER BY bucket_started_at)), 0) AS cpu_usage_micros,
			       COALESCE(GREATEST(0, gc_pause_total_micros - LAG(gc_pause_total_micros) OVER (ORDER BY bucket_started_at)), 0) AS gc_pause_micros
			FROM grouped
		)
		SELECT * FROM rates ORDER BY bucket_started_at`,
		stepSeconds, stepSeconds, component, instance, clusterID, from.UTC(), to.UTC()).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query platform instance trend: %w", err)
	}
	return rows, nil
}

type LeadershipEventPage struct {
	Items []PlatformLeadershipEvent
	Total int64
}

func (r *PlatformObservabilityRepo) LatestLeadershipEvents(
	ctx context.Context,
	since time.Time,
) ([]PlatformLeadershipEvent, error) {
	var rows []PlatformLeadershipEvent
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (component, cluster_id) *
		FROM kubesentry.platform_leadership_events
		WHERE changed_at >= ?
		ORDER BY component, cluster_id, changed_at DESC, id DESC`,
		since.UTC()).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query latest platform leadership events: %w", err)
	}
	return rows, nil
}

func (r *PlatformObservabilityRepo) LeadershipEvents(
	ctx context.Context,
	component string,
	clusterID *int64,
	from, to time.Time,
	page, pageSize int,
) (LeadershipEventPage, error) {
	query := r.db.WithContext(ctx).Model(&PlatformLeadershipEvent{}).
		Where("changed_at >= ? AND changed_at < ?", from.UTC(), to.UTC())
	if component != "" {
		query = query.Where("component = ?", component)
	}
	if clusterID != nil {
		query = query.Where("cluster_id = ?", *clusterID)
	}
	var result LeadershipEventPage
	if err := query.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("count leadership events: %w", err)
	}
	if err := query.Order("changed_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&result.Items).Error; err != nil {
		return result, fmt.Errorf("list leadership events: %w", err)
	}
	return result, nil
}

func (r *PlatformObservabilityRepo) Cleanup(ctx context.Context, now time.Time) (samples, events int64, err error) {
	samples, err = r.deleteInBatches(ctx, &PlatformComponentSample{},
		"bucket_started_at < ?", now.UTC().Add(-PlatformSampleRetention))
	if err != nil {
		return samples, 0, fmt.Errorf("clean platform samples: %w", err)
	}
	events, err = r.deleteInBatches(ctx, &PlatformLeadershipEvent{},
		"changed_at < ?", now.UTC().Add(-PlatformLeadershipRetention))
	if err != nil {
		return samples, events, fmt.Errorf("clean platform leadership events: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&PlatformObservabilitySettings{}).
		Where("id = 1").
		Updates(map[string]any{"last_cleanup_at": now.UTC(), "updated_at": now.UTC()}).Error; err != nil {
		return samples, events, fmt.Errorf("record platform cleanup: %w", err)
	}
	return samples, events, nil
}

func (r *PlatformObservabilityRepo) deleteInBatches(
	ctx context.Context,
	model any,
	where string,
	value time.Time,
) (int64, error) {
	var deleted int64
	for {
		var ids []int64
		if err := r.db.WithContext(ctx).Model(model).Select("id").
			Where(where, value).Order("id ASC").
			Limit(platformCleanupBatchSize).Scan(&ids).Error; err != nil {
			return deleted, err
		}
		if len(ids) == 0 {
			return deleted, nil
		}
		result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(model)
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if len(ids) < platformCleanupBatchSize {
			return deleted, nil
		}
	}
}

func DecodeDBStats(row PlatformComponentSample) map[string]any {
	raw, err := json.Marshal(row.DBStats.Data())
	if err != nil {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil || result == nil {
		return map[string]any{}
	}
	return result
}
