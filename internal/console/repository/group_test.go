package repository

import (
	"testing"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestDBRepoSatisfiesGroupRepository(t *testing.T) {
	var _ GroupRepository = (*dbrepo.GroupRepo)(nil)
}
