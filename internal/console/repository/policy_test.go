package repository

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// The whole point of this package is that internal/dbrepo satisfies these
// interfaces without a forwarding layer. If dbrepo's signature drifts, this
// test fails at compile time rather than at runtime in a handler.
func TestDBRepoSatisfiesPolicyRepository(t *testing.T) {
	var _ PolicyRepository = (*dbrepo.PolicyRepo)(nil)
}
