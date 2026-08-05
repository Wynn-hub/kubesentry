// Package repository declares the data-access interfaces the console's service
// layer depends on. The implementations live in internal/dbrepo (shared with
// cmd/gateway) — this package only narrows them to what the console uses, so
// the service layer can be tested against a fake and the handler layer cannot
// reach a repository at all.
package repository

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// PolicyRepository is the subset of dbrepo.PolicyRepo the console needs.
type PolicyRepository interface {
	ListPage(ctx context.Context, opts dbrepo.PolicyPageOptions) ([]dbrepo.Policy, int64, error)
}

// Compile-time proof that dbrepo satisfies the interface without a forwarding
// layer. Also asserted in policy_test.go so a signature drift surfaces as a
// test failure, not just a build error in some downstream package.
var _ PolicyRepository = (*dbrepo.PolicyRepo)(nil)
