package dbrepo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type GroupRepo struct{ db *gorm.DB }

func NewGroupRepo(db *gorm.DB) *GroupRepo { return &GroupRepo{db: db} }

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

func (r *GroupRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&PolicyGroup{}, id).Error
}
