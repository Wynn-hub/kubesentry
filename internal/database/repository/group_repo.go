package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type GroupRepo struct{ db *gorm.DB }

func NewGroupRepo(db *gorm.DB) *GroupRepo { return &GroupRepo{db: db} }

func (r *GroupRepo) WithTx(ctx context.Context, fn func(*GroupRepo, *DistributionRepo) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&GroupRepo{db: tx}, &DistributionRepo{db: tx}) })
}

func (r *GroupRepo) Create(ctx context.Context, g *PolicyGroup) error {
	return r.db.WithContext(ctx).Create(g).Error
}

func (r *GroupRepo) GetByName(ctx context.Context, name string) (*PolicyGroup, error) {
	var g PolicyGroup
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&g).Error; err != nil {
		return nil, fmt.Errorf("get policy group %q: %w", name, err)
	}
	return &g, nil
}

func (r *GroupRepo) List(ctx context.Context) ([]PolicyGroup, error) {
	var out []PolicyGroup
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list policy groups: %w", err)
	}
	return out, nil
}

func (r *GroupRepo) Update(ctx context.Context, g *PolicyGroup) error {
	return r.db.WithContext(ctx).Save(g).Error
}

// UpdateCAS applies fields to the PolicyGroup row id, but only if its
// updated_at still equals expectedUpdatedAt — see PolicyRepo.UpdateCAS.
func (r *GroupRepo) UpdateCAS(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any) error {
	result := r.db.WithContext(ctx).Model(&PolicyGroup{}).
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

func (r *GroupRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&PolicyGroup{}, id).Error
}
