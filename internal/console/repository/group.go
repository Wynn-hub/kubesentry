package repository

import (
	"context"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// GroupRepository is the subset of dbrepo.GroupRepo the policy domain needs:
// listing groups to compute which ones reference a policy.
type GroupRepository interface {
	List(ctx context.Context) ([]dbrepo.PolicyGroup, error)
}
type PolicyGroupRepository interface {
	GroupRepository
	GetByName(ctx context.Context, name string) (*dbrepo.PolicyGroup, error)
	Create(ctx context.Context, g *dbrepo.PolicyGroup) error
	UpdateCAS(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any) error
	Delete(ctx context.Context, id int64) error
}

var _ PolicyGroupRepository = (*dbrepo.GroupRepo)(nil)
