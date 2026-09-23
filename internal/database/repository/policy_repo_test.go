package repository_test

import (
	"context"
	"errors"
	"testing"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"gorm.io/datatypes"
)

const validRego = "package kubesentry\ndeny[msg]{false; msg:=\"x\"}"

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

func TestPolicyRepoListPageFiltersAndSorts(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPolicyRepo(db)
	ctx := context.Background()

	policies := []*dbrepo.Policy{
		{Name: "zz-page-policy-a", Description: "needle alpha", EnforcementMode: "enforce", CompileOK: true, Rego: validRego},
		{
			Name: "zz-page-policy-b", Description: "needle beta", EnforcementMode: "audit", CompileOK: false, Rego: validRego,
			Labels: datatypes.NewJSONType(map[string]string{"kubesentry.io/builtin": "true"}),
		},
		{Name: "zz-page-policy-c", Description: "needle gamma", EnforcementMode: "audit", CompileOK: true, Rego: validRego},
	}
	for _, policy := range policies {
		if err := repo.Create(ctx, policy); err != nil {
			t.Fatalf("create %s: %v", policy.Name, err)
		}
	}

	items, total, err := repo.ListPage(ctx, dbrepo.PolicyPageOptions{
		PageOptions: dbrepo.PageOptions{Limit: 1, Offset: 1, SortBy: "name"},
		Keyword:     "zz-page-policy-",
	})
	if err != nil {
		t.Fatalf("list page: %v", err)
	}
	if total != 3 || len(items) != 1 || items[0].Name != "zz-page-policy-b" {
		t.Fatalf("page = %+v total=%d, want zz-page-policy-b total=3", items, total)
	}

	filtered, total, err := repo.ListPage(ctx, dbrepo.PolicyPageOptions{
		PageOptions: dbrepo.PageOptions{Limit: 10, SortBy: "phase"},
		Sources:     []string{"custom"},
		Phases:      []string{"Ready"},
		Modes:       []string{"enforce"},
		Keyword:     "needle",
	})
	if err != nil {
		t.Fatalf("filtered page: %v", err)
	}
	if total != 1 || len(filtered) != 1 || filtered[0].Name != "zz-page-policy-a" {
		t.Fatalf("filtered = %+v total=%d", filtered, total)
	}

	if _, _, err := repo.ListPage(ctx, dbrepo.PolicyPageOptions{
		PageOptions: dbrepo.PageOptions{Limit: 10, SortBy: "unsafe"},
	}); err == nil {
		t.Fatal("expected invalid sort field to fail")
	}
}

// TestPolicyRepoWithTxRollsBackOnVersionConflict proves the Policy row and
// its PolicyVersion insert commit or roll back together: forcing
// CreateVersion to fail (a duplicate (policy_id, version) — see migration
// 0001's policy_versions_policy_version_key unique index) inside WithTx must
// not leave an orphaned Policy row behind. A subsequent create for the same
// name should succeed, not spuriously 409.
func TestPolicyRepoWithTxRollsBackOnVersionConflict(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPolicyRepo(db)
	ctx := context.Background()
	const name = "tx-rollback-policy"

	err := repo.WithTx(ctx, func(tx *dbrepo.PolicyRepo) error {
		p := &dbrepo.Policy{Name: name, Rego: validRego}
		if err := tx.Create(ctx, p); err != nil {
			return err
		}
		if err := tx.CreateVersion(ctx, &dbrepo.PolicyVersion{PolicyID: p.ID, Version: 1}); err != nil {
			return err
		}
		// Same (policy_id, version) again — violates the unique index and
		// forces the transaction to fail.
		return tx.CreateVersion(ctx, &dbrepo.PolicyVersion{PolicyID: p.ID, Version: 1})
	})
	if err == nil {
		t.Fatal("expected the transaction to fail on the duplicate version insert")
	}

	if _, err := repo.GetByName(ctx, name); err == nil {
		t.Fatal("policy row should have been rolled back, but it was found")
	}

	// Retrying the same create+version sequence must succeed cleanly — proof
	// there is no orphaned Policy row left behind by the failed attempt.
	err = repo.WithTx(ctx, func(tx *dbrepo.PolicyRepo) error {
		p := &dbrepo.Policy{Name: name, Rego: validRego}
		if err := tx.Create(ctx, p); err != nil {
			return err
		}
		return tx.CreateVersion(ctx, &dbrepo.PolicyVersion{PolicyID: p.ID, Version: 1})
	})
	if err != nil {
		t.Fatalf("retry after rollback should succeed, got: %v", err)
	}
}

// TestPolicyRepoUpdateCASRejectsStaleWriter proves the compare-and-swap
// actually closes the race a plain "read, compare, then unconditional Save"
// leaves open: two writers read the same row (same updated_at snapshot);
// the first writer's CAS succeeds, and the second writer — still holding the
// stale snapshot — must be rejected with ErrConflict instead of silently
// overwriting the first writer's change.
func TestPolicyRepoUpdateCASRejectsStaleWriter(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPolicyRepo(db)
	ctx := context.Background()

	p := &dbrepo.Policy{Name: "cas-policy", EnforcementMode: "audit", Rego: validRego}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Two "clients" read the same row.
	snapshot, err := repo.GetByName(ctx, "cas-policy")
	if err != nil {
		t.Fatal(err)
	}
	staleUpdatedAt := snapshot.UpdatedAt

	// First writer applies its change using the snapshot's updated_at.
	if err := repo.UpdateCAS(ctx, snapshot.ID, staleUpdatedAt, map[string]any{"enforcement_mode": "enforce"}); err != nil {
		t.Fatalf("first writer CAS: %v", err)
	}

	// Second writer, still holding the same stale snapshot, must be
	// rejected — not silently overwrite the first writer's change.
	err = repo.UpdateCAS(ctx, snapshot.ID, staleUpdatedAt, map[string]any{"enforcement_mode": "audit"})
	if !errors.Is(err, dbrepo.ErrConflict) {
		t.Fatalf("second writer CAS err = %v, want ErrConflict", err)
	}

	got, err := repo.GetByName(ctx, "cas-policy")
	if err != nil {
		t.Fatal(err)
	}
	if got.EnforcementMode != "enforce" {
		t.Fatalf("mode = %q, want %q (only the first writer's change should stick)", got.EnforcementMode, "enforce")
	}
}
