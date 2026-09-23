package dbrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/database/model"
)

type IncidentRepo struct{ db *gorm.DB }

func NewIncidentRepo(db *gorm.DB) *IncidentRepo { return &IncidentRepo{db: db} }

type IncidentPageOptions struct {
	PageOptions
	ClusterNames []string
	Components   []string
	Reasons      []string
	Statuses     []string
	From         *time.Time
	To           *time.Time
	Now          time.Time
}

func (r *IncidentRepo) Upsert(
	ctx context.Context,
	clusterID int64,
	clusterName string,
	incident admissionobs.Incident,
	receivedAt time.Time,
) error {
	if err := incident.Validate(); err != nil {
		return err
	}
	receivedAt = receivedAt.UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := AdmissionLinkIncident{
			ID: model.NewID(), ClusterID: clusterID, ClusterName: clusterName,
			IncidentKey: incident.IncidentKey, ReporterInstanceID: incident.ReporterInstanceID,
			Component: string(incident.Component), Reason: incident.Reason,
			StartedAt: incident.StartedAt.UTC(), LastSeenAt: incident.LastSeenAt.UTC(),
			AffectedCount: incident.AffectedCount, SampleEventIDs: pq.StringArray(incident.SampleEventIDs),
			Detail: incident.Detail, Closed: incident.Closed,
			FirstReceivedAt: receivedAt, LastReceivedAt: receivedAt,
		}
		created := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "cluster_id"}, {Name: "incident_key"}},
			DoNothing: true,
		}).Create(&row)
		if created.Error != nil || created.RowsAffected == 1 {
			return created.Error
		}

		// An absent-row SELECT ... FOR UPDATE cannot serialize two first reports.
		// Insert-or-ignore establishes the row atomically; only the loser reaches
		// this lock and merges its newer snapshot into that established row.
		var stored AdmissionLinkIncident
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("cluster_id = ? AND incident_key = ?", clusterID, incident.IncidentKey).
			First(&stored).Error; err != nil {
			return err
		}
		samples := admissionobs.MergeSampleEventIDs(stored.SampleEventIDs, incident.SampleEventIDs)
		updates := map[string]any{
			"cluster_name": clusterName, "reporter_instance_id": incident.ReporterInstanceID,
			"affected_count":   max(stored.AffectedCount, incident.AffectedCount),
			"last_seen_at":     maxTime(stored.LastSeenAt, incident.LastSeenAt.UTC()),
			"last_received_at": maxTime(stored.LastReceivedAt, receivedAt),
			"sample_event_ids": pq.StringArray(samples), "closed": stored.Closed || incident.Closed,
		}
		// Only a report that is at least as fresh as the stored member timestamp
		// may replace diagnostic detail; an out-of-order heartbeat cannot do so.
		if !incident.LastSeenAt.Before(stored.LastSeenAt) {
			updates["detail"] = incident.Detail
		}
		return tx.Model(&stored).Updates(updates).Error
	})
}

func (r *IncidentRepo) Get(ctx context.Context, id int64) (*AdmissionLinkIncident, error) {
	var incident AdmissionLinkIncident
	if err := r.db.WithContext(ctx).First(&incident, id).Error; err != nil {
		return nil, fmt.Errorf("get admission link incident %d: %w", id, err)
	}
	return &incident, nil
}

func (r *IncidentRepo) ListPage(ctx context.Context, options IncidentPageOptions) ([]AdmissionLinkIncident, int64, error) {
	query := r.db.WithContext(ctx).Model(&AdmissionLinkIncident{})
	if len(options.ClusterNames) > 0 {
		query = query.Where("cluster_name IN ?", options.ClusterNames)
	}
	if len(options.Components) > 0 {
		query = query.Where("component IN ?", options.Components)
	}
	if len(options.Reasons) > 0 {
		query = query.Where("reason IN ?", options.Reasons)
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if len(options.Statuses) > 0 {
		fresh := now.Add(-admissionobs.ActiveFreshness)
		if predicate, args := statusPredicate(options.Statuses, fresh); predicate != "" {
			query = query.Where(predicate, args...)
		}
	}
	if options.From != nil {
		query = query.Where("last_seen_at >= ?", options.From.UTC())
	}
	if options.To != nil {
		query = query.Where("started_at <= ?", options.To.UTC())
	}
	items, total, err := queryPage[AdmissionLinkIncident](query, options.PageOptions,
		map[string]string{"startedAt": "started_at", "lastSeenAt": "last_seen_at", "affectedCount": "affected_count"},
		"last_seen_at DESC, id DESC")
	if err != nil {
		return nil, 0, fmt.Errorf("list admission link incidents: %w", err)
	}
	return items, total, nil
}

func statusPredicate(statuses []string, fresh time.Time) (string, []any) {
	predicates := make([]string, 0, len(statuses))
	args := make([]any, 0, len(statuses))
	for _, status := range statuses {
		switch status {
		case "active":
			predicates = append(predicates, "(closed = FALSE AND last_received_at >= ?)")
			args = append(args, fresh)
		case "interrupted":
			predicates = append(predicates, "(closed = FALSE AND last_received_at < ?)")
			args = append(args, fresh)
		case "closed":
			predicates = append(predicates, "closed = TRUE")
		}
	}
	if len(predicates) == 0 {
		return "", nil
	}
	return "(" + strings.Join(predicates, " OR ") + ")", args
}

func (r *IncidentRepo) Cleanup(ctx context.Context, now time.Time, retentionDays int) (int64, error) {
	if retentionDays < 1 || retentionDays > 365 {
		return 0, errors.New("incident retention days must be between 1 and 365")
	}
	cutoff := now.UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	result := r.db.WithContext(ctx).Where("last_received_at < ?", cutoff).Delete(&AdmissionLinkIncident{})
	return result.RowsAffected, result.Error
}

func maxTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
