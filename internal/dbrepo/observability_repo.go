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

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

const (
	observabilityCleanupBatchSize  = 1000
	ObservationCursorIdleRetention = 24 * time.Hour
)

type ObservabilityRepo struct {
	db *gorm.DB
}

func NewObservabilityRepo(db *gorm.DB) *ObservabilityRepo {
	return &ObservabilityRepo{db: db}
}

func (r *ObservabilityRepo) GetSettings(ctx context.Context) (*PlatformObservabilitySettings, error) {
	var settings PlatformObservabilitySettings
	if err := r.db.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return nil, fmt.Errorf("get platform observability settings: %w", err)
	}
	return &settings, nil
}

func (r *ObservabilityRepo) UpdateSettings(ctx context.Context, retentionDays int) error {
	if retentionDays < 1 || retentionDays > 365 {
		return fmt.Errorf("retention days must be between 1 and 365")
	}
	result := r.db.WithContext(ctx).Model(&PlatformObservabilitySettings{}).
		Where("id = 1").
		Updates(map[string]any{"retention_days": retentionDays, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return fmt.Errorf("update platform observability settings: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.New("platform observability settings row not found")
	}
	return nil
}

type ApplyObservationResult struct {
	Duplicate     bool
	BaselineReset bool
	SequenceGap   int64
	BucketStarted time.Time
}

type ObservationBucketDelta struct {
	BucketStartedAt        time.Time
	ClusterID              int64
	ClusterName            string
	Component              admissionobs.Component
	Snapshot               admissionobs.Snapshot
	ObservationWriteFailed int64
	SequenceGapCount       int64
	BaselineResetCount     int64
	SeriesStartedCount     int64
	QueueDepthMax          int64
	QueueCapacity          int64
	LastObservedAt         time.Time
}

func (r *ObservabilityRepo) ApplyMemberReport(ctx context.Context, clusterID int64, clusterName string, report admissionobs.Report, receivedAt time.Time) (ApplyObservationResult, error) {
	report.Snapshot = admissionobs.NormalizeSnapshot(report.Snapshot)
	if err := report.Validate(); err != nil {
		return ApplyObservationResult{}, err
	}
	if clusterID <= 0 || clusterName == "" {
		return ApplyObservationResult{}, errors.New("trusted cluster identity is required")
	}
	receivedAt = receivedAt.UTC()
	bucketStarted := observationBucketTime(report.ObservedAt, receivedAt)
	result := ApplyObservationResult{BucketStarted: bucketStarted}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cursor ObservationReportCursor
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cluster_id = ? AND series_id = ?", clusterID, report.SeriesID).
			Take(&cursor).Error

		currentSnapshotJSON, marshalErr := json.Marshal(report.Snapshot)
		if marshalErr != nil {
			return fmt.Errorf("marshal observation snapshot: %w", marshalErr)
		}

		delta := ObservationBucketDelta{
			BucketStartedAt: bucketStarted,
			ClusterID:       clusterID,
			ClusterName:     clusterName,
			Component:       report.Component,
			QueueDepthMax:   report.QueueDepthMax,
			QueueCapacity:   report.QueueCapacity,
			LastObservedAt:  receivedAt,
		}

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			cursor = ObservationReportCursor{
				ID:              dbmodel.NewID(),
				ClusterID:       clusterID,
				SeriesID:        report.SeriesID,
				Component:       string(report.Component),
				LastSequence:    int64(report.Sequence),
				CounterSnapshot: datatypes.JSON(currentSnapshotJSON),
				LastActiveAt:    receivedAt,
			}
			if report.Sequence == 1 {
				delta.Snapshot = report.Snapshot
				delta.SeriesStartedCount = 1
			} else {
				delta.BaselineResetCount = 1
				result.BaselineReset = true
			}
			if err := tx.Create(&cursor).Error; err != nil {
				return fmt.Errorf("create observation cursor: %w", err)
			}
		case err != nil:
			return fmt.Errorf("lock observation cursor: %w", err)
		default:
			if report.Component != admissionobs.Component(cursor.Component) {
				return errors.New("series component changed")
			}
			if int64(report.Sequence) <= cursor.LastSequence {
				result.Duplicate = true
				return nil
			}
			if int64(report.Sequence) > cursor.LastSequence+1 {
				delta.SequenceGapCount = int64(report.Sequence) - cursor.LastSequence - 1
				result.SequenceGap = delta.SequenceGapCount
			}
			var previous admissionobs.Snapshot
			if err := json.Unmarshal(cursor.CounterSnapshot, &previous); err != nil {
				return fmt.Errorf("decode observation cursor snapshot: %w", err)
			}
			snapshotDelta, deltaErr := report.Snapshot.Delta(previous)
			if deltaErr != nil {
				delta.BaselineResetCount = 1
				delta.SequenceGapCount = 0
				result.BaselineReset = true
				result.SequenceGap = 0
			} else {
				delta.Snapshot = snapshotDelta
			}
			if err := tx.Model(&cursor).Updates(map[string]any{
				"last_sequence":    report.Sequence,
				"counter_snapshot": datatypes.JSON(currentSnapshotJSON),
				"last_active_at":   receivedAt,
				"updated_at":       receivedAt,
			}).Error; err != nil {
				return fmt.Errorf("update observation cursor: %w", err)
			}
		}

		if err := upsertObservationBucket(tx, delta); err != nil {
			return err
		}
		if err := tx.Model(&Cluster{}).
			Where("id = ? AND observability_onboarded_at IS NULL", clusterID).
			Update("observability_onboarded_at", receivedAt).Error; err != nil {
			return fmt.Errorf("mark cluster observability onboarding: %w", err)
		}
		return nil
	})
	return result, err
}

func observationBucketTime(observedAt, receivedAt time.Time) time.Time {
	observedAt = observedAt.UTC()
	receivedAt = receivedAt.UTC()
	delta := observedAt.Sub(receivedAt)
	if delta < -5*time.Minute || delta > 5*time.Minute {
		return receivedAt.Truncate(time.Minute)
	}
	return observedAt.Truncate(time.Minute)
}

func (r *ObservabilityRepo) UpsertBucket(ctx context.Context, delta ObservationBucketDelta) error {
	if !delta.Component.ValidBucket() {
		return fmt.Errorf("invalid observation component %q", delta.Component)
	}
	if delta.BucketStartedAt.IsZero() || delta.LastObservedAt.IsZero() {
		return errors.New("bucket and observed timestamps are required")
	}
	return upsertObservationBucket(r.db.WithContext(ctx), delta)
}

func upsertObservationBucket(db *gorm.DB, delta ObservationBucketDelta) error {
	counters := delta.Snapshot.Counters
	bucket := AdmissionObservationBucket{
		ID:                     dbmodel.NewID(),
		BucketStartedAt:        delta.BucketStartedAt.UTC().Truncate(time.Minute),
		ClusterID:              delta.ClusterID,
		ClusterName:            delta.ClusterName,
		Component:              string(delta.Component),
		GeneratedCount:         counters.Generated,
		ReceivedCount:          counters.Received,
		ForwardedCount:         counters.Forwarded,
		WrittenCount:           counters.Written,
		DuplicateCount:         counters.Duplicate,
		PolicyFilteredCount:    counters.PolicyFiltered,
		RetentionElapsedCount:  counters.RetentionElapsed,
		QueueRejectedCount:     counters.QueueRejected,
		RetryCount:             counters.Retry,
		DroppedCount:           counters.Dropped,
		RetryExhaustedCount:    counters.RetryExhausted,
		BatchFailureCount:      counters.BatchFailure,
		AuthFailureCount:       counters.AuthFailure,
		ACKTimeoutCount:        counters.ACKTimeout,
		ReportDroppedCount:     counters.ReportDropped,
		ObservationWriteFailed: delta.ObservationWriteFailed,
		SequenceGapCount:       delta.SequenceGapCount,
		BaselineResetCount:     delta.BaselineResetCount,
		SeriesStartedCount:     delta.SeriesStartedCount,
		CleanupRunCount:        counters.CleanupRun,
		CleanupDeletedCount:    counters.CleanupDeleted,
		CleanupFailureCount:    counters.CleanupFailure,
		QueueDepthMax:          delta.QueueDepthMax,
		QueueCapacity:          delta.QueueCapacity,
		FaultReasonCounts:      datatypes.NewJSONType(nonNilCounts(delta.Snapshot.FaultReasonCounts)),
		PressureReasonCounts:   datatypes.NewJSONType(nonNilCounts(delta.Snapshot.PressureReasonCounts)),
		FilterReasonCounts:     datatypes.NewJSONType(nonNilCounts(delta.Snapshot.FilterReasonCounts)),
		LastObservedAt:         delta.LastObservedAt.UTC(),
	}
	add := func(column string) clause.Expr {
		return gorm.Expr("admission_observation_buckets." + column + " + EXCLUDED." + column)
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "bucket_started_at"}, {Name: "cluster_id"}, {Name: "component"}},
		DoUpdates: clause.Assignments(map[string]any{
			"cluster_name":             gorm.Expr("CASE WHEN EXCLUDED.cluster_name = '' THEN admission_observation_buckets.cluster_name ELSE EXCLUDED.cluster_name END"),
			"generated_count":          add("generated_count"),
			"received_count":           add("received_count"),
			"forwarded_count":          add("forwarded_count"),
			"written_count":            add("written_count"),
			"duplicate_count":          add("duplicate_count"),
			"policy_filtered_count":    add("policy_filtered_count"),
			"retention_elapsed_count":  add("retention_elapsed_count"),
			"queue_rejected_count":     add("queue_rejected_count"),
			"retry_count":              add("retry_count"),
			"dropped_count":            add("dropped_count"),
			"retry_exhausted_count":    add("retry_exhausted_count"),
			"batch_failure_count":      add("batch_failure_count"),
			"auth_failure_count":       add("auth_failure_count"),
			"ack_timeout_count":        add("ack_timeout_count"),
			"report_dropped_count":     add("report_dropped_count"),
			"observation_write_failed": add("observation_write_failed"),
			"sequence_gap_count":       add("sequence_gap_count"),
			"baseline_reset_count":     add("baseline_reset_count"),
			"series_started_count":     add("series_started_count"),
			"cleanup_run_count":        add("cleanup_run_count"),
			"cleanup_deleted_count":    add("cleanup_deleted_count"),
			"cleanup_failure_count":    add("cleanup_failure_count"),
			"queue_depth_max":          gorm.Expr("GREATEST(admission_observation_buckets.queue_depth_max, EXCLUDED.queue_depth_max)"),
			"queue_capacity":           gorm.Expr("GREATEST(admission_observation_buckets.queue_capacity, EXCLUDED.queue_capacity)"),
			"fault_reason_counts":      gorm.Expr("kubesentry.jsonb_counter_add(admission_observation_buckets.fault_reason_counts, EXCLUDED.fault_reason_counts)"),
			"pressure_reason_counts":   gorm.Expr("kubesentry.jsonb_counter_add(admission_observation_buckets.pressure_reason_counts, EXCLUDED.pressure_reason_counts)"),
			"filter_reason_counts":     gorm.Expr("kubesentry.jsonb_counter_add(admission_observation_buckets.filter_reason_counts, EXCLUDED.filter_reason_counts)"),
			"last_observed_at":         gorm.Expr("GREATEST(admission_observation_buckets.last_observed_at, EXCLUDED.last_observed_at)"),
			"updated_at":               time.Now().UTC(),
		}),
	}).Create(&bucket).Error
	if err != nil {
		return fmt.Errorf("upsert admission observation bucket: %w", err)
	}
	return nil
}

func nonNilCounts(counts map[string]int64) map[string]int64 {
	if counts == nil {
		return map[string]int64{}
	}
	return counts
}

type ObservationAggregate struct {
	ClusterID              int64
	ClusterName            string
	Component              string
	admissionobs.Counters  `gorm:"embedded"`
	ObservationWriteFailed int64
	SequenceGapCount       int64
	BaselineResetCount     int64
	SeriesStartedCount     int64
	QueueDepthMax          int64
	QueueCapacity          int64
	LastObservedAt         *time.Time
	FaultReasonCounts      map[string]int64
	PressureReasonCounts   map[string]int64
	FilterReasonCounts     map[string]int64
}

func (r *ObservabilityRepo) AggregateClusters(ctx context.Context, from, to time.Time) ([]ObservationAggregate, error) {
	selectColumns := `
		cluster_id,
		MAX(cluster_name) AS cluster_name,
		component,
		SUM(generated_count) AS generated,
		SUM(received_count) AS received,
		SUM(forwarded_count) AS forwarded,
		SUM(written_count) AS written,
		SUM(duplicate_count) AS duplicate,
		SUM(policy_filtered_count) AS policy_filtered,
		SUM(retention_elapsed_count) AS retention_elapsed,
		SUM(queue_rejected_count) AS queue_rejected,
		SUM(retry_count) AS retry,
		SUM(dropped_count) AS dropped,
		SUM(retry_exhausted_count) AS retry_exhausted,
		SUM(batch_failure_count) AS batch_failure,
		SUM(auth_failure_count) AS auth_failure,
		SUM(ack_timeout_count) AS ack_timeout,
		SUM(report_dropped_count) AS report_dropped,
		SUM(cleanup_run_count) AS cleanup_run,
		SUM(cleanup_deleted_count) AS cleanup_deleted,
		SUM(cleanup_failure_count) AS cleanup_failure,
		SUM(observation_write_failed) AS observation_write_failed,
		SUM(sequence_gap_count) AS sequence_gap_count,
		SUM(baseline_reset_count) AS baseline_reset_count,
		SUM(series_started_count) AS series_started_count,
		MAX(queue_depth_max) AS queue_depth_max,
		MAX(queue_capacity) AS queue_capacity,
		MAX(last_observed_at) AS last_observed_at`
	var result []ObservationAggregate
	err := r.db.WithContext(ctx).Model(&AdmissionObservationBucket{}).
		Select(selectColumns).
		Where("cluster_id <> 0 AND bucket_started_at >= ? AND bucket_started_at < ?", from.UTC(), to.UTC()).
		Group("cluster_id, component").
		Order("cluster_id, component").
		Scan(&result).Error
	if err != nil {
		return nil, fmt.Errorf("aggregate cluster admission observations: %w", err)
	}
	if err := r.attachClusterReasons(ctx, result, from, to); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *ObservabilityRepo) attachClusterReasons(ctx context.Context, aggregates []ObservationAggregate, from, to time.Time) error {
	type reasonRow struct {
		ClusterID int64
		Component string
		Reason    string
		Count     int64
	}
	index := make(map[string]*ObservationAggregate, len(aggregates))
	for i := range aggregates {
		key := fmt.Sprintf("%d/%s", aggregates[i].ClusterID, aggregates[i].Component)
		index[key] = &aggregates[i]
		aggregates[i].FaultReasonCounts = map[string]int64{}
		aggregates[i].PressureReasonCounts = map[string]int64{}
		aggregates[i].FilterReasonCounts = map[string]int64{}
	}
	for _, reasonColumn := range []struct {
		column string
		assign func(*ObservationAggregate) map[string]int64
	}{
		{"fault_reason_counts", func(aggregate *ObservationAggregate) map[string]int64 { return aggregate.FaultReasonCounts }},
		{"pressure_reason_counts", func(aggregate *ObservationAggregate) map[string]int64 { return aggregate.PressureReasonCounts }},
		{"filter_reason_counts", func(aggregate *ObservationAggregate) map[string]int64 { return aggregate.FilterReasonCounts }},
	} {
		var rows []reasonRow
		sql := fmt.Sprintf(`
			SELECT cluster_id, component, reason.key AS reason,
			       SUM(reason.value::TEXT::BIGINT) AS count
			FROM kubesentry.admission_observation_buckets,
			     LATERAL jsonb_each(%s) AS reason
			WHERE cluster_id <> 0
			  AND bucket_started_at >= ? AND bucket_started_at < ?
			GROUP BY cluster_id, component, reason.key`, reasonColumn.column)
		if err := r.db.WithContext(ctx).Raw(sql, from.UTC(), to.UTC()).Scan(&rows).Error; err != nil {
			return fmt.Errorf("aggregate cluster %s: %w", reasonColumn.column, err)
		}
		for _, row := range rows {
			if aggregate := index[fmt.Sprintf("%d/%s", row.ClusterID, row.Component)]; aggregate != nil {
				reasonColumn.assign(aggregate)[row.Reason] = row.Count
			}
		}
	}
	return nil
}

type ObservationTrendRow struct {
	BucketStartedAt       time.Time
	Component             string
	admissionobs.Counters `gorm:"embedded"`
}

func (r *ObservabilityRepo) Trend(
	ctx context.Context,
	clusterID *int64,
	from, to time.Time,
	step time.Duration,
) ([]ObservationTrendRow, error) {
	if step < time.Minute || step%time.Minute != 0 {
		return nil, errors.New("observation trend step must be whole minutes")
	}
	query := r.db.WithContext(ctx).Model(&AdmissionObservationBucket{}).
		Where("bucket_started_at >= ? AND bucket_started_at < ?", from.UTC(), to.UTC())
	if clusterID == nil {
		query = query.Where("cluster_id <> 0")
	} else {
		query = query.Where("cluster_id = ?", *clusterID)
	}
	interval := fmt.Sprintf("%d minutes", int(step/time.Minute))
	bucketExpression := fmt.Sprintf(
		"date_bin(INTERVAL '%s', bucket_started_at, TIMESTAMPTZ '1970-01-01 00:00:00+00')",
		interval,
	)
	var rows []ObservationTrendRow
	err := query.Select(bucketExpression + ` AS bucket_started_at,
			component,
			SUM(generated_count) AS generated,
			SUM(received_count) AS received,
			SUM(forwarded_count) AS forwarded,
			SUM(written_count) AS written,
			SUM(policy_filtered_count) AS policy_filtered,
			SUM(retention_elapsed_count) AS retention_elapsed,
			SUM(queue_rejected_count) AS queue_rejected,
			SUM(retry_count) AS retry,
			SUM(dropped_count) AS dropped,
			SUM(retry_exhausted_count) AS retry_exhausted,
			SUM(batch_failure_count) AS batch_failure,
			SUM(auth_failure_count) AS auth_failure,
			SUM(ack_timeout_count) AS ack_timeout,
			SUM(report_dropped_count) AS report_dropped`).
		Group(bucketExpression + ", component").
		Order("bucket_started_at, component").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query admission observation trend: %w", err)
	}
	return rows, nil
}

func (r *ObservabilityRepo) SustainedPressure(ctx context.Context, from, to time.Time) (map[int64]bool, error) {
	type pressureRow struct {
		ClusterID int64
	}
	var rows []pressureRow
	err := r.db.WithContext(ctx).Raw(`
		WITH minute_pressure AS (
			SELECT cluster_id, bucket_started_at,
			       BOOL_OR(queue_capacity > 0 AND queue_depth_max * 100 >= queue_capacity * 80) AS high_water,
			       BOOL_OR(queue_rejected_count > 0) AS rejected
			FROM kubesentry.admission_observation_buckets
			WHERE cluster_id <> 0
			  AND bucket_started_at >= ? AND bucket_started_at < ?
			GROUP BY cluster_id, bucket_started_at
		),
		with_previous AS (
			SELECT cluster_id, bucket_started_at, high_water, rejected,
			       LAG(bucket_started_at) OVER (PARTITION BY cluster_id ORDER BY bucket_started_at) AS previous_bucket,
			       LAG(high_water) OVER (PARTITION BY cluster_id ORDER BY bucket_started_at) AS previous_high_water,
			       LAG(rejected) OVER (PARTITION BY cluster_id ORDER BY bucket_started_at) AS previous_rejected
			FROM minute_pressure
		)
		SELECT DISTINCT cluster_id
		FROM with_previous
		WHERE bucket_started_at - previous_bucket = INTERVAL '1 minute'
		  AND ((high_water AND previous_high_water) OR (rejected AND previous_rejected))`,
		from.UTC(), to.UTC()).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query sustained observation pressure: %w", err)
	}
	result := make(map[int64]bool, len(rows))
	for _, row := range rows {
		result[row.ClusterID] = true
	}
	return result, nil
}

func (r *ObservabilityRepo) Aggregate(ctx context.Context, clusterID *int64, from, to time.Time) ([]ObservationAggregate, error) {
	query := r.db.WithContext(ctx).Model(&AdmissionObservationBucket{}).
		Where("bucket_started_at >= ? AND bucket_started_at < ?", from.UTC(), to.UTC())
	if clusterID != nil {
		query = query.Where("cluster_id = ?", *clusterID)
	}
	selectColumns := `
		component,
		SUM(generated_count) AS generated,
		SUM(received_count) AS received,
		SUM(forwarded_count) AS forwarded,
		SUM(written_count) AS written,
		SUM(duplicate_count) AS duplicate,
		SUM(policy_filtered_count) AS policy_filtered,
		SUM(retention_elapsed_count) AS retention_elapsed,
		SUM(queue_rejected_count) AS queue_rejected,
		SUM(retry_count) AS retry,
		SUM(dropped_count) AS dropped,
		SUM(retry_exhausted_count) AS retry_exhausted,
		SUM(batch_failure_count) AS batch_failure,
		SUM(auth_failure_count) AS auth_failure,
		SUM(ack_timeout_count) AS ack_timeout,
		SUM(report_dropped_count) AS report_dropped,
		SUM(cleanup_run_count) AS cleanup_run,
		SUM(cleanup_deleted_count) AS cleanup_deleted,
		SUM(cleanup_failure_count) AS cleanup_failure,
		SUM(observation_write_failed) AS observation_write_failed,
		SUM(sequence_gap_count) AS sequence_gap_count,
		SUM(baseline_reset_count) AS baseline_reset_count,
		SUM(series_started_count) AS series_started_count,
		MAX(queue_depth_max) AS queue_depth_max,
		MAX(queue_capacity) AS queue_capacity,
		MAX(last_observed_at) AS last_observed_at`
	var result []ObservationAggregate
	if err := query.Select(selectColumns).Group("component").Order("component").Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("aggregate admission observations: %w", err)
	}
	for index := range result {
		fault, err := r.aggregateReasons(ctx, clusterID, from, to, result[index].Component, "fault_reason_counts")
		if err != nil {
			return nil, err
		}
		pressure, err := r.aggregateReasons(ctx, clusterID, from, to, result[index].Component, "pressure_reason_counts")
		if err != nil {
			return nil, err
		}
		filter, err := r.aggregateReasons(ctx, clusterID, from, to, result[index].Component, "filter_reason_counts")
		if err != nil {
			return nil, err
		}
		result[index].FaultReasonCounts = fault
		result[index].PressureReasonCounts = pressure
		result[index].FilterReasonCounts = filter
	}
	return result, nil
}

func (r *ObservabilityRepo) aggregateReasons(ctx context.Context, clusterID *int64, from, to time.Time, component, column string) (map[string]int64, error) {
	where := "bucket_started_at >= ? AND bucket_started_at < ? AND component = ?"
	args := []any{from.UTC(), to.UTC(), component}
	if clusterID != nil {
		where += " AND cluster_id = ?"
		args = append(args, *clusterID)
	}
	type reasonRow struct {
		Reason string
		Count  int64
	}
	var rows []reasonRow
	sql := fmt.Sprintf(`
		SELECT reason.key AS reason, SUM(reason.value::TEXT::BIGINT) AS count
		FROM kubesentry.admission_observation_buckets,
		     LATERAL jsonb_each(%s) AS reason
		WHERE %s
		GROUP BY reason.key`, column, where)
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("aggregate %s: %w", column, err)
	}
	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		result[row.Reason] = row.Count
	}
	return result, nil
}

func (r *ObservabilityRepo) LatestByComponent(ctx context.Context, clusterID int64) (map[string]time.Time, error) {
	type row struct {
		Component      string
		LastObservedAt time.Time
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&AdmissionObservationBucket{}).
		Select("component, MAX(last_observed_at) AS last_observed_at").
		Where("cluster_id = ? AND component IN ?", clusterID, []string{string(admissionobs.ComponentWebhook), string(admissionobs.ComponentOperator)}).
		Group("component").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query observation freshness: %w", err)
	}
	result := make(map[string]time.Time, len(rows))
	for _, row := range rows {
		result[row.Component] = row.LastObservedAt
	}
	return result, nil
}

func (r *ObservabilityRepo) Cleanup(ctx context.Context, now time.Time) (bucketsDeleted, cursorsDeleted int64, err error) {
	settings, err := r.GetSettings(ctx)
	if err != nil {
		return 0, 0, err
	}
	bucketsDeleted, err = r.deleteInBatches(ctx, &AdmissionObservationBucket{}, "bucket_started_at < ?", now.UTC().Add(-time.Duration(settings.RetentionDays)*24*time.Hour))
	if err != nil {
		return bucketsDeleted, 0, fmt.Errorf("clean observation buckets: %w", err)
	}
	cursorsDeleted, err = r.deleteInBatches(ctx, &ObservationReportCursor{}, "last_active_at < ?", now.UTC().Add(-ObservationCursorIdleRetention))
	if err != nil {
		return bucketsDeleted, cursorsDeleted, fmt.Errorf("clean observation cursors: %w", err)
	}
	return bucketsDeleted, cursorsDeleted, nil
}

func (r *ObservabilityRepo) deleteInBatches(ctx context.Context, model any, where string, value time.Time) (int64, error) {
	var deleted int64
	var cursor int64
	for {
		var ids []int64
		if err := r.db.WithContext(ctx).Model(model).Select("id").
			Where(where, value).Where("id > ?", cursor).
			Order("id ASC").Limit(observabilityCleanupBatchSize).Scan(&ids).Error; err != nil {
			return deleted, err
		}
		if len(ids) == 0 {
			return deleted, nil
		}
		cursor = ids[len(ids)-1]
		result := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(model)
		if result.Error != nil {
			return deleted, result.Error
		}
		deleted += result.RowsAffected
		if len(ids) < observabilityCleanupBatchSize {
			return deleted, nil
		}
	}
}
