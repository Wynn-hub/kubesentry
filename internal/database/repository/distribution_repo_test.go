package repository_test

import (
	"context"
	"testing"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
	"gorm.io/datatypes"
)

func TestDistributionRepoUpdateCASSucceedsOnFreshRead(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewDistributionRepo(db)
	ctx := context.Background()

	d := &dbrepo.PolicyDistribution{
		Name: "e2e-dist-cas",
		Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{PolicyGroups: []string{"g1"}}),
	}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}

	newSpec := datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{PolicyGroups: []string{"g1", "g2"}})
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
		Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{PolicyGroups: []string{"g1"}}),
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
	fresh.Spec = datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{PolicyGroups: []string{"g1", "concurrent"}})
	if err := repo.Update(ctx, fresh); err != nil {
		t.Fatalf("concurrent update: %v", err)
	}

	// The stale caller's CAS must now fail.
	newSpec := datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{PolicyGroups: []string{"g1", "stale-write"}})
	if err := repo.UpdateCAS(ctx, d.ID, staleUpdatedAt, map[string]any{"spec": newSpec}); err == nil {
		t.Fatal("expected ErrConflict on a stale updated_at, got nil")
	}
}

func TestDistributionRepoListPage(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewDistributionRepo(db)
	ctx := context.Background()

	for _, name := range []string{"zz-page-dist-a", "zz-page-dist-b"} {
		if err := repo.Create(ctx, &dbrepo.PolicyDistribution{
			Name: name,
			Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{PolicyGroups: []string{"default"}}),
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	items, total, err := repo.ListPage(ctx, dbrepo.PageOptions{
		Limit: 1, SortBy: "name", Desc: true,
	})
	if err != nil {
		t.Fatalf("list page: %v", err)
	}
	if total < 2 || len(items) != 1 || items[0].Name != "zz-page-dist-b" {
		t.Fatalf("page = %+v total=%d, want zz-page-dist-b", items, total)
	}

	if _, _, err := repo.ListPage(ctx, dbrepo.PageOptions{Limit: 10, SortBy: "unsafe"}); err == nil {
		t.Fatal("expected invalid sort field to fail")
	}
}
