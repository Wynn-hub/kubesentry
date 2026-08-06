package repository

import (
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestDBRepoSatisfiesGroupRepository(t *testing.T) {
	var _ GroupRepository = (*dbrepo.GroupRepo)(nil)
}
