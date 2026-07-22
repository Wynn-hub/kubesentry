package dbrepo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type InstallRecordRepo struct{ db *gorm.DB }

func NewInstallRecordRepo(db *gorm.DB) *InstallRecordRepo { return &InstallRecordRepo{db: db} }

func (r *InstallRecordRepo) Create(ctx context.Context, rec *ClusterInstallRecord) error {
	return r.db.WithContext(ctx).Create(rec).Error
}

// List returns every install record, newest first.
func (r *InstallRecordRepo) List(ctx context.Context) ([]ClusterInstallRecord, error) {
	var out []ClusterInstallRecord
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list install records: %w", err)
	}
	return out, nil
}

func (r *InstallRecordRepo) ListPage(ctx context.Context, options PageOptions) ([]ClusterInstallRecord, int64, error) {
	stableOrder := "id ASC"
	if options.Desc {
		stableOrder = "id DESC"
	}
	items, total, err := queryPage[ClusterInstallRecord](
		r.db.WithContext(ctx).Model(&ClusterInstallRecord{}),
		options,
		map[string]string{
			"clusterName": "cluster_name",
			"namespace":   "namespace",
			"createdAt":   "created_at",
		},
		stableOrder,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list install record page: %w", err)
	}
	return items, total, nil
}

// LatestForCluster returns the most recent record for a cluster name, or nil
// if none exists — a missing history is a normal state (a cluster that has
// never had a command generated for it), not an error.
func (r *InstallRecordRepo) LatestForCluster(ctx context.Context, clusterName string) (*ClusterInstallRecord, error) {
	var rec ClusterInstallRecord
	err := r.db.WithContext(ctx).Where("cluster_name = ?", clusterName).
		Order("created_at DESC").First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest install record for %q: %w", clusterName, err)
	}
	return &rec, nil
}
