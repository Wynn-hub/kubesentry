package dbrepo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type ExceptionRepo struct{ db *gorm.DB }

func NewExceptionRepo(db *gorm.DB) *ExceptionRepo { return &ExceptionRepo{db: db} }

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

func (r *ExceptionRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&PolicyException{}, id).Error
}
