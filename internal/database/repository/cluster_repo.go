package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ClusterRepo struct{ db *gorm.DB }

type ClusterPhaseCounts struct {
	Total       int64
	Online      int64
	Offline     int64
	Registering int64
}

func NewClusterRepo(db *gorm.DB) *ClusterRepo { return &ClusterRepo{db: db} }

// WithTx binds the cluster, install-record, and uninstall-task repositories to
// one transaction for console lifecycle operations that perform multiple writes.
func (r *ClusterRepo) WithTx(ctx context.Context, fn func(*ClusterRepo, *InstallRecordRepo, *UninstallTaskRepo) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&ClusterRepo{db: tx}, &InstallRecordRepo{db: tx}, &UninstallTaskRepo{db: tx})
	})
}

func (r *ClusterRepo) Create(ctx context.Context, c *Cluster) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *ClusterRepo) GetByName(ctx context.Context, name string) (*Cluster, error) {
	var c Cluster
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&c).Error; err != nil {
		return nil, fmt.Errorf("get cluster %q: %w", name, err)
	}
	return &c, nil
}

func (r *ClusterRepo) List(ctx context.Context) ([]Cluster, error) {
	var out []Cluster
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	return out, nil
}

func (r *ClusterRepo) ListPage(ctx context.Context, options PageOptions) ([]Cluster, int64, error) {
	items, total, err := queryPage[Cluster](
		r.db.WithContext(ctx).Model(&Cluster{}),
		options,
		map[string]string{
			"name":            "name",
			"phase":           "phase",
			"operatorVersion": "operator_version",
			"lastHeartbeatAt": "last_heartbeat_at",
			"createdAt":       "created_at",
		},
		"name ASC",
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list cluster page: %w", err)
	}
	return items, total, nil
}

func (r *ClusterRepo) CountByPhase(ctx context.Context) (ClusterPhaseCounts, error) {
	base := r.db.WithContext(ctx).Model(&Cluster{})
	var counts ClusterPhaseCounts
	if err := base.Count(&counts.Total).Error; err != nil {
		return ClusterPhaseCounts{}, fmt.Errorf("count clusters: %w", err)
	}

	var rows []struct {
		Phase string
		Count int64
	}
	if err := base.Select("phase, COUNT(*) AS count").Group("phase").Scan(&rows).Error; err != nil {
		return ClusterPhaseCounts{}, fmt.Errorf("count clusters by phase: %w", err)
	}
	for _, row := range rows {
		switch row.Phase {
		case "Online":
			counts.Online = row.Count
		case "Offline":
			counts.Offline = row.Count
		case "Registering":
			counts.Registering = row.Count
		}
	}
	return counts, nil
}

func (r *ClusterRepo) Update(ctx context.Context, c *Cluster) error {
	return r.db.WithContext(ctx).Save(c).Error
}

// UpdateHeartbeat touches only the columns StatusFlusher owns. A full-row
// Save() here would race with Register (which mutates AuthTokenHash /
// RegTokenHash / ClusterUID / Phase on the same row): flushOnce works off a
// List() snapshot that can be stale by the time it writes back, and Save()
// would stomp Register's changes with that stale copy.
func (r *ClusterRepo) UpdateHeartbeat(ctx context.Context, id int64, phase string, lastHeartbeatAt *time.Time, operatorVersion string) error {
	return r.db.WithContext(ctx).Model(&Cluster{}).Where("id = ?", id).Updates(map[string]any{
		"phase":             phase,
		"last_heartbeat_at": lastHeartbeatAt,
		"operator_version":  operatorVersion,
	}).Error
}

// ClusterInventory is the member-reported description of a cluster.
type ClusterInventory struct {
	KubernetesVersion string
	APIServerAddress  string
	NodeCount         int
	ReadyNodeCount    int
}

// UpdateInventory writes only the member-reported inventory columns, following
// the same column-scoped rule as UpdateHeartbeat above. Empty strings are
// skipped so a member that temporarily cannot read a value does not blank out
// what was previously reported; node counts are written together (a genuine
// zero-node reading is meaningless, so both are skipped when NodeCount is 0).
func (r *ClusterRepo) UpdateInventory(ctx context.Context, id int64, inv ClusterInventory) error {
	updates := map[string]any{}
	if inv.KubernetesVersion != "" {
		updates["kubernetes_version"] = inv.KubernetesVersion
	}
	if inv.APIServerAddress != "" {
		updates["api_server_address"] = inv.APIServerAddress
	}
	if inv.NodeCount > 0 {
		updates["node_count"] = inv.NodeCount
		updates["ready_node_count"] = inv.ReadyNodeCount
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Cluster{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateLabels overwrites only the labels column. Like UpdateHeartbeat
// above, this must never be a full-row Save() — console-triggered label
// edits run concurrently with StatusFlusher's heartbeat writes to the same
// row, and a stale-snapshot Save() would silently revert whichever side
// wrote last.
func (r *ClusterRepo) UpdateLabels(ctx context.Context, id int64, labels map[string]string) error {
	return r.db.WithContext(ctx).Model(&Cluster{}).Where("id = ?", id).
		Updates(map[string]any{"labels": datatypes.NewJSONType(labels)}).Error
}

// UpdateRegToken overwrites only reg_token_hash — used when reissuing a
// lost one-time registration token for a cluster still in Registering.
func (r *ClusterRepo) UpdateRegToken(ctx context.Context, id int64, hash string) error {
	return r.db.WithContext(ctx).Model(&Cluster{}).Where("id = ?", id).
		Updates(map[string]any{"reg_token_hash": hash}).Error
}

// UpdatePhase changes only lifecycle state so task reports cannot clobber
// auth credentials, labels, or heartbeat fields written by other actors.
func (r *ClusterRepo) UpdatePhase(ctx context.Context, id int64, phase string) error {
	return r.db.WithContext(ctx).Model(&Cluster{}).Where("id = ?", id).
		Updates(map[string]any{"phase": phase}).Error
}

func (r *ClusterRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Cluster{}, id).Error
}
