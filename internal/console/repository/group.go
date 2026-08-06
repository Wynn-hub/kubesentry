package repository

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// GroupRepository is the subset of dbrepo.GroupRepo the policy domain needs:
// listing groups to compute which ones reference a policy.
type GroupRepository interface {
	List(ctx context.Context) ([]dbrepo.PolicyGroup, error)
}

var _ GroupRepository = (*dbrepo.GroupRepo)(nil)
