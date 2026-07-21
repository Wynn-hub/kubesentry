package dbrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type DistributionRepo struct{ db *gorm.DB }

func NewDistributionRepo(db *gorm.DB) *DistributionRepo { return &DistributionRepo{db: db} }

func (r *DistributionRepo) Create(ctx context.Context, d *PolicyDistribution) error {
	return r.db.WithContext(ctx).Create(d).Error
}

func (r *DistributionRepo) GetByName(ctx context.Context, name string) (*PolicyDistribution, error) {
	var d PolicyDistribution
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&d).Error; err != nil {
		return nil, fmt.Errorf("get policy distribution %q: %w", name, err)
	}
	return &d, nil
}

func (r *DistributionRepo) List(ctx context.Context) ([]PolicyDistribution, error) {
	var out []PolicyDistribution
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list policy distributions: %w", err)
	}
	return out, nil
}

func (r *DistributionRepo) Update(ctx context.Context, d *PolicyDistribution) error {
	return r.db.WithContext(ctx).Save(d).Error
}

// UpdateCAS applies fields to the PolicyDistribution row id, but only if its
// updated_at still equals expectedUpdatedAt — see GroupRepo.UpdateCAS.
func (r *DistributionRepo) UpdateCAS(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any) error {
	result := r.db.WithContext(ctx).Model(&PolicyDistribution{}).
		Where("id = ? AND updated_at = ?", id, expectedUpdatedAt).
		Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

func (r *DistributionRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&PolicyDistribution{}, id).Error
}

// UpsertClusterStatus records the per-cluster apply status for a
// distribution, matching on (distribution_id, cluster_id).
func (r *DistributionRepo) UpsertClusterStatus(ctx context.Context, s *DistributionClusterStatus) error {
	return r.db.WithContext(ctx).
		Where("distribution_id = ? AND cluster_id = ?", s.DistributionID, s.ClusterID).
		Assign(DistributionClusterStatus{
			Status: s.Status, AppliedVersionHash: s.AppliedVersionHash, LastError: s.LastError,
		}).
		FirstOrCreate(s).Error
}

// GetClusterStatus returns the current per-cluster status entry, or nil if
// none has been recorded yet (a distribution just started targeting this
// cluster).
func (r *DistributionRepo) GetClusterStatus(ctx context.Context, distributionID, clusterID int64) (*DistributionClusterStatus, error) {
	var s DistributionClusterStatus
	err := r.db.WithContext(ctx).
		Where("distribution_id = ? AND cluster_id = ?", distributionID, clusterID).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get distribution cluster status: %w", err)
	}
	return &s, nil
}

// DeleteClusterStatus removes a per-cluster status entry (the distribution no
// longer targets this cluster). A no-op if no entry exists.
func (r *DistributionRepo) DeleteClusterStatus(ctx context.Context, distributionID, clusterID int64) error {
	return r.db.WithContext(ctx).
		Where("distribution_id = ? AND cluster_id = ?", distributionID, clusterID).
		Delete(&DistributionClusterStatus{}).Error
}
