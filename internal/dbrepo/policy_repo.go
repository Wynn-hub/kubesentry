package dbrepo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type PolicyRepo struct{ db *gorm.DB }

func NewPolicyRepo(db *gorm.DB) *PolicyRepo { return &PolicyRepo{db: db} }

func (r *PolicyRepo) Create(ctx context.Context, p *Policy) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *PolicyRepo) GetByName(ctx context.Context, name string) (*Policy, error) {
	var p Policy
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&p).Error; err != nil {
		return nil, fmt.Errorf("get policy %q: %w", name, err)
	}
	return &p, nil
}

func (r *PolicyRepo) List(ctx context.Context) ([]Policy, error) {
	var out []Policy
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	return out, nil
}

func (r *PolicyRepo) Update(ctx context.Context, p *Policy) error {
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *PolicyRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Policy{}, id).Error
}

// CreateVersion inserts an immutable policy_versions snapshot in the same
// transaction as updating policies (spec 二: PolicyVersion 语义).
func (r *PolicyRepo) CreateVersion(ctx context.Context, v *PolicyVersion) error {
	return r.db.WithContext(ctx).Create(v).Error
}

// ListVersions returns all versions for a policy, newest first.
func (r *PolicyRepo) ListVersions(ctx context.Context, policyID int64) ([]PolicyVersion, error) {
	var out []PolicyVersion
	err := r.db.WithContext(ctx).Where("policy_id = ?", policyID).Order("version DESC").Find(&out).Error
	return out, err
}

// PruneVersions deletes versions beyond keepLast (oldest first), matching
// the existing PolicyReconciler history-limit behavior.
func (r *PolicyRepo) PruneVersions(ctx context.Context, policyID int64, keepLast int) error {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&PolicyVersion{}).
		Where("policy_id = ?", policyID).
		Order("version DESC").
		Offset(keepLast).
		Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return err
	}
	return r.db.WithContext(ctx).Delete(&PolicyVersion{}, ids).Error
}
