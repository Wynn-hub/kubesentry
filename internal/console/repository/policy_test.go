package repository

import (
	"testing"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"gorm.io/gorm"
)

// The whole point of this package is that internal/database/repository satisfies these
// interfaces without a forwarding layer. If dbrepo's signature drifts, this
// test fails at compile time rather than at runtime in a handler.
func TestDBRepoSatisfiesPolicyRepository(t *testing.T) {
	var _ PolicyRepository = (*dbrepo.PolicyRepo)(nil)
}

func TestNewBuildsConsoleRepositoryBundle(t *testing.T) {
	repos := New(&gorm.DB{})
	if repos.Policies == nil || repos.Groups == nil || repos.Exceptions == nil ||
		repos.Distributions == nil || repos.Clusters == nil || repos.InstallRecords == nil ||
		repos.UninstallTasks == nil || repos.Incidents == nil || repos.Admission == nil ||
		repos.Platform == nil || repos.ExceptionUOW == nil || repos.ClusterUOW == nil {
		t.Fatal("New returned an incomplete repository bundle")
	}
	if repos.Groups != repos.PolicyGroups {
		t.Fatal("group repository instances differ")
	}
}
