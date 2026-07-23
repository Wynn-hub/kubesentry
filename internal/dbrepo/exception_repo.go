package dbrepo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type ExceptionRepo struct{ db *gorm.DB }

func NewExceptionRepo(db *gorm.DB) *ExceptionRepo { return &ExceptionRepo{db: db} }

func (r *ExceptionRepo) WithTx(ctx context.Context, fn func(*ExceptionRepo, *DistributionRepo) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(&ExceptionRepo{db: tx}, &DistributionRepo{db: tx}) })
}

func (r *ExceptionRepo) Create(ctx context.Context, e *PolicyException) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *ExceptionRepo) GetByName(ctx context.Context, name string) (*PolicyException, error) {
	var e PolicyException
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&e).Error; err != nil {
		return nil, fmt.Errorf("get policy exception %q: %w", name, err)
	}
	return &e, nil
}

func (r *ExceptionRepo) List(ctx context.Context) ([]PolicyException, error) {
	var out []PolicyException
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list policy exceptions: %w", err)
	}
	return out, nil
}

func (r *ExceptionRepo) Update(ctx context.Context, e *PolicyException) error {
	return r.db.WithContext(ctx).Save(e).Error
}

// UpdateCAS applies fields to the PolicyException row id, but only if its
// updated_at still equals expectedUpdatedAt — see PolicyRepo.UpdateCAS.
func (r *ExceptionRepo) UpdateCAS(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any) error {
	result := r.db.WithContext(ctx).Model(&PolicyException{}).
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

func (r *ExceptionRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&PolicyException{}, id).Error
}
