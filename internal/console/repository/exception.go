package repository

import (
	"context"
	"time"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

type ExceptionRepository interface {
	List(context.Context) ([]dbrepo.PolicyException, error)
	GetByName(context.Context, string) (*dbrepo.PolicyException, error)
	Create(context.Context, *dbrepo.PolicyException) error
	UpdateCAS(context.Context, int64, time.Time, map[string]any) error
	Delete(context.Context, int64) error
}

var _ ExceptionRepository = (*dbrepo.ExceptionRepo)(nil)
