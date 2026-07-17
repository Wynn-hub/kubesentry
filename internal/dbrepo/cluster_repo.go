package dbrepo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type ClusterRepo struct{ db *gorm.DB }

func NewClusterRepo(db *gorm.DB) *ClusterRepo { return &ClusterRepo{db: db} }

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

func (r *ClusterRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Cluster{}, id).Error
}
