package dbrepo

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Wynn-hub/kubesentry/internal/admissionpolicy"
)

type AdmissionRepo struct{ db *gorm.DB }

type AdmissionEventPageOptions struct {
	PageOptions
	ClusterNames []string
	Verdicts     []string
	PolicyName   string
	Namespaces   []string
	GVKs         []string
	ResourceName string
	Username     string
	From         *time.Time
	To           *time.Time
}

func NewAdmissionRepo(db *gorm.DB) *AdmissionRepo { return &AdmissionRepo{db: db} }

func (r *AdmissionRepo) GetSettings(ctx context.Context) (*AdmissionEventSettings, error) {
	var settings AdmissionEventSettings
	if err := r.db.WithContext(ctx).First(&settings, 1).Error; err != nil {
		return nil, fmt.Errorf("get admission event settings: %w", err)
	}
	return &settings, nil
}

func (r *AdmissionRepo) ListPolicies(ctx context.Context) ([]AdmissionStoragePolicy, error) {
	var policies []AdmissionStoragePolicy
	err := r.db.WithContext(ctx).Order("priority ASC").Find(&policies).Error
	return policies, err
}

func (r *AdmissionRepo) CreatePolicy(ctx context.Context, policy *AdmissionStoragePolicy) error {
	return r.db.WithContext(ctx).Create(policy).Error
}

func (r *AdmissionRepo) UpdateSettingsAndPolicies(ctx context.Context, settings *AdmissionEventSettings, policies []AdmissionStoragePolicy) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"default_retention_days": settings.DefaultRetentionDays,
			"retry_enabled":          settings.RetryEnabled,
			"max_retry_attempts":     settings.MaxRetryAttempts,
			"queue_capacity":         settings.QueueCapacity,
			"observation_enabled":    settings.ObservationEnabled,
			"storage_policy_version": gorm.Expr("storage_policy_version + 1"),
		}
		// Preserve the existing value for in-process callers built against the
		// earlier settings shape. The HTTP handler always supplies this field,
		// while a zero value here means "not specified", never a valid retention.
		if settings.IncidentRetentionDays > 0 {
			updates["incident_retention_days"] = settings.IncidentRetentionDays
		}
		if err := tx.Model(&AdmissionEventSettings{}).Where("id = ?", settings.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&AdmissionStoragePolicy{}).Error; err != nil {
			return err
		}
		for i := range policies {
			normalizeAdmissionStoragePolicy(&policies[i])
			if err := tx.Create(&policies[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func normalizeAdmissionStoragePolicy(policy *AdmissionStoragePolicy) {
	if policy.MatchMode == "" {
		policy.MatchMode = AdmissionMatchModeInclude
	}
	if policy.Verdicts == nil {
		policy.Verdicts = pq.StringArray{}
	}
	if policy.Clusters == nil {
		policy.Clusters = pq.StringArray{}
	}
	if policy.GVKs == nil {
		policy.GVKs = pq.StringArray{}
	}
	if policy.Namespaces == nil {
		policy.Namespaces = pq.StringArray{}
	}
}

func (r *AdmissionRepo) CreateEvent(ctx context.Context, event *AdmissionEvent) error {
	normalizeAdmissionEvent(event)
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *AdmissionRepo) GetEvent(ctx context.Context, id int64) (*AdmissionEvent, error) {
	var event AdmissionEvent
	if err := r.db.WithContext(ctx).First(&event, id).Error; err != nil {
		return nil, fmt.Errorf("get admission event %d: %w", id, err)
	}
	return &event, nil
}

func (r *AdmissionRepo) EventDatabaseIDs(ctx context.Context, eventIDs []string) (map[string]int64, error) {
	result := make(map[string]int64, len(eventIDs))
	if len(eventIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		ID      int64
		EventID string
	}
	if err := r.db.WithContext(ctx).Model(&AdmissionEvent{}).
		Select("id, event_id").Where("event_id IN ?", eventIDs).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("resolve admission event ids: %w", err)
	}
	for _, row := range rows {
		result[row.EventID] = row.ID
	}
	return result, nil
}

func (r *AdmissionRepo) ListEventsPage(ctx context.Context, options AdmissionEventPageOptions) ([]AdmissionEvent, int64, error) {
	query := r.db.WithContext(ctx).Model(&AdmissionEvent{})
	if len(options.ClusterNames) > 0 {
		query = query.Where("cluster_name IN ?", options.ClusterNames)
	}
	if len(options.Verdicts) > 0 {
		query = query.Where("verdict IN ?", options.Verdicts)
	}
	if options.PolicyName != "" {
		query = query.Where("policy_names @> ?", pq.StringArray{options.PolicyName})
	}
	if len(options.Namespaces) > 0 {
		query = query.Where("namespace IN ?", options.Namespaces)
	}
	if len(options.GVKs) > 0 {
		// Normalized GVR: core group ("") renders as bare resource ("pods"),
		// non-core as "<group>/<resource>" ("apps/deployments") — matching the
		// canonical storage-policy format (no leading slash for core group).
		query = query.Where(
			"CASE WHEN api_group = '' THEN resource ELSE api_group || '/' || resource END IN ?",
			options.GVKs,
		)
	}
	if options.ResourceName != "" {
		query = query.Where("resource_name = ?", options.ResourceName)
	}
	if options.Username != "" {
		query = query.Where("username = ?", options.Username)
	}
	if options.From != nil {
		query = query.Where("occurred_at >= ?", *options.From)
	}
	if options.To != nil {
		query = query.Where("occurred_at <= ?", *options.To)
	}
	items, total, err := queryPage[AdmissionEvent](query, options.PageOptions, map[string]string{"occurredAt": "occurred_at", "createdAt": "created_at", "verdict": "verdict"}, "id DESC")
	if err != nil {
		return nil, 0, fmt.Errorf("list admission events: %w", err)
	}
	return items, total, nil
}

type AdmissionEventTrendBucket struct {
	Bucket  time.Time
	Verdict string
	Count   int64
}

func (r *AdmissionRepo) Trend(ctx context.Context, from, to time.Time, hourly bool) ([]AdmissionEventTrendBucket, error) {
	unit := "day"
	if hourly {
		unit = "hour"
	}
	var rows []AdmissionEventTrendBucket
	err := r.db.WithContext(ctx).Model(&AdmissionEvent{}).Select("date_trunc(?, occurred_at) AS bucket, verdict, COUNT(*) AS count", unit).
		Where("occurred_at >= ? AND occurred_at <= ?", from, to).Group("bucket, verdict").Order("bucket ASC").Scan(&rows).Error
	return rows, err
}

// CreateEventsIdempotent commits an entire accepted batch before its caller
// acknowledges it. A duplicate event ID is already durable and is therefore
// treated as successfully accepted.
func (r *AdmissionRepo) CreateEventsIdempotent(ctx context.Context, events []AdmissionEvent) error {
	_, _, err := r.CreateEventsIdempotentCount(ctx, events)
	return err
}

// CreateEventsIdempotentCount is the instrumented form used by the gateway:
// written counts newly durable rows, while duplicates counts rows that already
// existed under the event_id unique key.
func (r *AdmissionRepo) CreateEventsIdempotentCount(ctx context.Context, events []AdmissionEvent) (written, duplicates int64, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range events {
			normalizeAdmissionEvent(&events[i])
			result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Create(&events[i])
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				duplicates++
			} else {
				written++
			}
		}
		return nil
	})
	return written, duplicates, err
}

// normalizeAdmissionEvent ensures optional protobuf fields cannot bypass the
// non-NULL SQL defaults. pq.StringArray(nil) and datatypes.JSON(nil) are both
// sent to PostgreSQL as NULL when GORM includes them in an INSERT.
func normalizeAdmissionEvent(event *AdmissionEvent) {
	if event.UserGroups == nil {
		event.UserGroups = pq.StringArray{}
	}
	if event.PolicyNames == nil {
		event.PolicyNames = pq.StringArray{}
	}
	if event.PolicyGroupNames == nil {
		event.PolicyGroupNames = pq.StringArray{}
	}
	event.Violations = normalizeAdmissionJSON(event.Violations, []byte("[]"))
	event.MetadataSnapshot = normalizeAdmissionJSON(event.MetadataSnapshot, []byte("{}"))
	event.SpecSnapshot = normalizeAdmissionJSON(event.SpecSnapshot, []byte("{}"))
}

func normalizeAdmissionJSON(value datatypes.JSON, fallback []byte) datatypes.JSON {
	if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return datatypes.JSON(fallback)
	}
	return value
}

func (r *AdmissionRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("created_at <= ?", before).Unscoped().Delete(&AdmissionEvent{})
	return result.RowsAffected, result.Error
}

// admissionCleanupBatchSize bounds how many candidate rows one cleanup
// iteration holds in memory. admission_events is a high-write table, so a
// single unbounded SELECT is not an option.
const admissionCleanupBatchSize = 1000

// minAdmissionRetention is the shortest retention the settings allow (see the
// default_retention_days CHECK). Routine cleanup skips anything newer than
// this, since no configuration can expire it yet; rows that stopped matching
// the rule set within that window are collected by CleanupAll instead.
const minAdmissionRetention = 24 * time.Hour

// admissionCleanupCandidate is the column projection cleanup needs: the
// storage-rule match inputs plus the retention timestamp. It deliberately
// omits metadata_snapshot/spec_snapshot — reading those JSONB columns for
// every historical row is the expensive part of a scan, and cleanup never
// looks at their contents.
type admissionCleanupCandidate struct {
	ID          int64     `gorm:"column:id"`
	ClusterName string    `gorm:"column:cluster_name"`
	Verdict     string    `gorm:"column:verdict"`
	APIGroup    string    `gorm:"column:api_group"`
	Resource    string    `gorm:"column:resource"`
	Namespace   string    `gorm:"column:namespace"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// Cleanup applies the currently active configuration to historical events.
// Retention is intentionally not persisted per row: shortening the global
// retention period, or narrowing the rule set so an event no longer matches any
// rule, therefore affects existing data on the next run.
//
// This is the routine daily pass, so it ignores rows younger than the shortest
// allowed retention. Use CleanupAll right after a configuration change to also
// drop newly-unmatched rows inside that window.
func (r *AdmissionRepo) Cleanup(ctx context.Context, now time.Time) (int64, error) {
	return r.cleanup(ctx, now, now.Add(-minAdmissionRetention))
}

// CleanupAll is Cleanup over the whole table, including events younger than the
// shortest allowed retention. Narrowing the rule set makes matching rows
// immediately ineligible for storage at any age, and the routine pass would
// otherwise leave the most recent day of them behind until it aged out —
// looking, from the console, exactly like the new rule had not taken effect.
func (r *AdmissionRepo) CleanupAll(ctx context.Context, now time.Time) (int64, error) {
	return r.cleanup(ctx, now, now)
}

// cleanup scans candidates older than notAfter in primary-key batches and
// deletes the ones the current configuration no longer keeps. Paging by `id >
// cursor` rather than OFFSET keeps each batch's cost constant and is stable
// under the concurrent inserts this table always sees.
func (r *AdmissionRepo) cleanup(ctx context.Context, now, notAfter time.Time) (int64, error) {
	settings, err := r.GetSettings(ctx)
	if err != nil {
		return 0, err
	}
	policies, err := r.ListPolicies(ctx)
	if err != nil {
		return 0, err
	}
	rules := admissionRulesOf(policies)
	retention := time.Duration(settings.DefaultRetentionDays) * 24 * time.Hour

	var deleted int64
	var cursor int64
	for {
		var candidates []admissionCleanupCandidate
		err := r.db.WithContext(ctx).Model(&AdmissionEvent{}).
			Select("id", "cluster_name", "verdict", "api_group", "resource", "namespace", "created_at").
			Where("created_at <= ? AND id > ?", notAfter, cursor).
			Order("id ASC").Limit(admissionCleanupBatchSize).
			Scan(&candidates).Error
		if err != nil {
			return deleted, fmt.Errorf("scan admission event cleanup candidates: %w", err)
		}
		if len(candidates) == 0 {
			return deleted, nil
		}
		cursor = candidates[len(candidates)-1].ID

		expired := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			if !admissionpolicy.ShouldStore(rules, candidate.storageEvent()) {
				expired = append(expired, candidate.ID)
				continue
			}
			if !candidate.CreatedAt.Add(retention).After(now) {
				expired = append(expired, candidate.ID)
			}
		}
		if len(expired) > 0 {
			result := r.db.WithContext(ctx).Unscoped().Delete(&AdmissionEvent{}, expired)
			if result.Error != nil {
				return deleted, fmt.Errorf("delete expired admission events: %w", result.Error)
			}
			deleted += result.RowsAffected
		}
		if len(candidates) < admissionCleanupBatchSize {
			return deleted, nil
		}
	}
}

func (c admissionCleanupCandidate) storageEvent() admissionpolicy.Event {
	return admissionpolicy.Event{
		Verdict:     c.Verdict,
		ClusterName: c.ClusterName,
		APIGroup:    c.APIGroup,
		Resource:    c.Resource,
		Namespace:   c.Namespace,
	}
}
