// Package repository declares the data-access interfaces the console's service
// layer depends on. The implementations live in internal/dbrepo (shared with
// cmd/gateway) — this package only narrows them to what the console uses, so
// the service layer can be tested against a fake and the handler layer cannot
// reach a repository at all.
package repository

import (
	"context"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// PolicyRepository is the subset of dbrepo.PolicyRepo the console needs.
// It deliberately does not declare WithTx: transaction composition is pushed
// down into dbrepo (CreateWithVersion/UpdateWithVersion) so the service layer
// never holds a concrete *dbrepo.PolicyRepo.
type PolicyRepository interface {
	ListPage(ctx context.Context, opts dbrepo.PolicyPageOptions) ([]dbrepo.Policy, int64, error)
	List(ctx context.Context) ([]dbrepo.Policy, error)
	GetByName(ctx context.Context, name string) (*dbrepo.Policy, error)
	CreateWithVersion(ctx context.Context, p *dbrepo.Policy, v *dbrepo.PolicyVersion) error
	UpdateWithVersion(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any, v *dbrepo.PolicyVersion) error
	Delete(ctx context.Context, id int64) error
	ListVersions(ctx context.Context, policyID int64) ([]dbrepo.PolicyVersion, error)
}

// Compile-time proof that dbrepo satisfies the interface without a forwarding
// layer. Also asserted in policy_test.go so a signature drift surfaces as a
// test failure, not just a build error in some downstream package.
var _ PolicyRepository = (*dbrepo.PolicyRepo)(nil)
