package dbrepo

import (
	"context"
	"fmt"

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

func (r *ClusterRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&Cluster{}, id).Error
}
