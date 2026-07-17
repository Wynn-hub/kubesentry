package dbrepo_test

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestPolicyRepoVersionLifecycle(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPolicyRepo(db)
	ctx := context.Background()

	p := &dbrepo.Policy{Name: "test-policy", Rego: "package kubesentry\ndeny[msg]{false; msg:=\"x\"}"}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("create policy: %v", err)
	}

	for v := 1; v <= 5; v++ {
		if err := repo.CreateVersion(ctx, &dbrepo.PolicyVersion{PolicyID: p.ID, Version: v}); err != nil {
			t.Fatalf("create version %d: %v", v, err)
		}
	}
	if err := repo.PruneVersions(ctx, p.ID, 3); err != nil {
		t.Fatalf("prune: %v", err)
	}
	versions, err := repo.ListVersions(ctx, p.ID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions after pruning to keepLast=3, got %d", len(versions))
	}
	if versions[0].Version != 5 {
		t.Errorf("expected newest-first order, got version %d first", versions[0].Version)
	}
}
