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
		if err := tx.Model(&AdmissionEventSettings{}).Where("id = ?", settings.ID).Updates(map[string]any{
			"default_retention_days": settings.DefaultRetentionDays,
			"storage_policy_version": gorm.Expr("storage_policy_version + 1"),
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&AdmissionStoragePolicy{}).Error; err != nil {
			return err
		}
		for i := range policies {
			if err := tx.Create(&policies[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range events {
			normalizeAdmissionEvent(&events[i])
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Create(&events[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

// Cleanup applies the currently active configuration to historical events.
// Retention is intentionally not persisted per row: shortening a policy or
// changing it to discard therefore affects existing data on the next run.
func (r *AdmissionRepo) Cleanup(ctx context.Context, now time.Time) (int64, error) {
	settings, err := r.GetSettings(ctx)
	if err != nil {
		return 0, err
	}
	policies, err := r.ListPolicies(ctx)
	if err != nil {
		return 0, err
	}
	var events []AdmissionEvent
	if err := r.db.WithContext(ctx).Where("created_at <= ?", now.Add(-24*time.Hour)).Find(&events).Error; err != nil {
		return 0, err
	}
	ids := make([]int64, 0)
	for _, event := range events {
		days := settings.DefaultRetentionDays
		if policy := MatchAdmissionStoragePolicy(policies, event); policy != nil {
			if policy.Action == AdmissionStorageActionDiscard {
				ids = append(ids, event.ID)
				continue
			}
			if policy.RetentionDays != nil {
				days = *policy.RetentionDays
			}
		}
		if !event.CreatedAt.Add(time.Duration(days) * 24 * time.Hour).After(now) {
			ids = append(ids, event.ID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Unscoped().Delete(&AdmissionEvent{}, ids)
	return result.RowsAffected, result.Error
}
