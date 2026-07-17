package dbrepo

import (
	"context"
	"fmt"

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
