package dbrepo_test

import (
	"context"
	"testing"

	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

func TestClusterRepoCreateAndGetByName(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	ctx := context.Background()

	c := &dbrepo.Cluster{Name: "local", Phase: "Registering"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("expected snowflake ID to be assigned after Create")
	}

	got, err := repo.GetByName(ctx, "local")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.Phase != "Registering" {
		t.Errorf("phase = %q, want Registering", got.Phase)
	}
}

func TestClusterRepoGetByNameNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	if _, err := repo.GetByName(context.Background(), "does-not-exist"); err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestClusterRepoListExcludesSoftDeleted(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	ctx := context.Background()

	c := &dbrepo.Cluster{Name: "to-delete"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, item := range list {
		if item.Name == "to-delete" {
			t.Fatal("expected soft-deleted cluster to be excluded from List")
		}
	}
}

func TestClusterRepoRecreateAfterSoftDeleteSameName(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	ctx := context.Background()

	first := &dbrepo.Cluster{Name: "reused-name"}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := repo.Delete(ctx, first.ID); err != nil {
		t.Fatalf("delete first: %v", err)
	}
	second := &dbrepo.Cluster{Name: "reused-name"}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("expected recreate with same name after soft delete to succeed: %v", err)
	}
}
