package dbrepo_test

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"gorm.io/datatypes"
)

func TestDistributionRepoUpdateCASSucceedsOnFreshRead(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewDistributionRepo(db)
	ctx := context.Background()

	d := &dbrepo.PolicyDistribution{
		Name: "e2e-dist-cas",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyDistributionSpec{PolicyGroups: []string{"g1"}}),
	}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}

	newSpec := datatypes.NewJSONType(v1alpha1.PolicyDistributionSpec{PolicyGroups: []string{"g1", "g2"}})
	if err := repo.UpdateCAS(ctx, d.ID, d.UpdatedAt, map[string]any{"spec": newSpec}); err != nil {
		t.Fatalf("update cas: %v", err)
	}

	got, err := repo.GetByName(ctx, "e2e-dist-cas")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if len(got.Spec.Data().PolicyGroups) != 2 {
		t.Fatalf("policyGroups = %+v, want 2 entries", got.Spec.Data().PolicyGroups)
	}
}

func TestDistributionRepoUpdateCASFailsOnStaleRead(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewDistributionRepo(db)
	ctx := context.Background()

	d := &dbrepo.PolicyDistribution{
		Name: "e2e-dist-stale",
		Spec: datatypes.NewJSONType(v1alpha1.PolicyDistributionSpec{PolicyGroups: []string{"g1"}}),
	}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}
	staleUpdatedAt := d.UpdatedAt

	// A concurrent write lands first.
	fresh, err := repo.GetByName(ctx, "e2e-dist-stale")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	fresh.Spec = datatypes.NewJSONType(v1alpha1.PolicyDistributionSpec{PolicyGroups: []string{"g1", "concurrent"}})
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("concurrent update: %v", err)
	}

	// The stale caller's CAS must now fail.
	newSpec := datatypes.NewJSONType(v1alpha1.PolicyDistributionSpec{PolicyGroups: []string{"g1", "stale-write"}})
	if err := repo.UpdateCAS(ctx, d.ID, staleUpdatedAt, map[string]any{"spec": newSpec}); err == nil {
		t.Fatal("expected ErrConflict on a stale updated_at, got nil")
	}
}
