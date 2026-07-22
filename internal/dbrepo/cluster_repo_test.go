package dbrepo_test

import (
	"context"
	"testing"
	"time"

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

func TestClusterRepoListPageAndCountByPhase(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	ctx := context.Background()

	for _, cluster := range []*dbrepo.Cluster{
		{Name: "page-cluster-a", Phase: "Online"},
		{Name: "page-cluster-b", Phase: "Offline"},
		{Name: "page-cluster-c", Phase: "Registering"},
	} {
		if err := repo.Create(ctx, cluster); err != nil {
			t.Fatalf("create %s: %v", cluster.Name, err)
		}
	}

	items, total, err := repo.ListPage(ctx, dbrepo.PageOptions{
		Limit: 1, Offset: 1, SortBy: "name",
	})
	if err != nil {
		t.Fatalf("list page: %v", err)
	}
	if total != 3 || len(items) != 1 || items[0].Name != "page-cluster-b" {
		t.Fatalf("page = %+v total=%d, want page-cluster-b total=3", items, total)
	}

	empty, total, err := repo.ListPage(ctx, dbrepo.PageOptions{
		Limit: 10, Offset: 100, SortBy: "name",
	})
	if err != nil || total != 3 || len(empty) != 0 {
		t.Fatalf("empty page = %+v total=%d err=%v", empty, total, err)
	}

	counts, err := repo.CountByPhase(ctx)
	if err != nil {
		t.Fatalf("count by phase: %v", err)
	}
	if counts.Total != 3 || counts.Online != 1 || counts.Offline != 1 || counts.Registering != 1 {
		t.Fatalf("counts = %+v", counts)
	}

	if _, _, err := repo.ListPage(ctx, dbrepo.PageOptions{Limit: 10, SortBy: "unsafe"}); err == nil {
		t.Fatal("expected invalid sort field to fail")
	}
}

// TestClusterRepoUpdateHeartbeatDoesNotClobberAuthFields reproduces the
// StatusFlusher/Register race at a logic level: StatusFlusher.flushOnce
// lists clusters, then (in the old buggy code) does a full-struct Save()
// back using that in-memory copy. If Register committed AuthTokenHash /
// cleared RegTokenHash in between, that stale Save() would stomp them back.
// UpdateHeartbeat only ever touches phase/last_heartbeat_at/operator_version
// columns, so it must leave AuthTokenHash/RegTokenHash/ClusterUID exactly as
// Register left them, regardless of what a stale caller believes those
// fields are.
func TestClusterRepoUpdateHeartbeatDoesNotClobberAuthFields(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	ctx := context.Background()

	c := &dbrepo.Cluster{Name: "prod", Phase: "Registering", RegTokenHash: "reg-hash"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Snapshot the pre-registration row — this stands in for the stale
	// in-memory copy StatusFlusher.flushOnce's List() would have captured
	// right before a concurrent Register call landed.
	stale, err := repo.GetByName(ctx, "prod")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}

	// Simulate Register (reading fresh, as the real handler does): commits
	// AuthTokenHash + ClusterUID, consumes the one-time RegTokenHash.
	registered, err := repo.GetByName(ctx, "prod")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	registered.AuthTokenHash = "auth-hash"
	registered.ClusterUID = "uid-1"
	registered.RegTokenHash = ""
	if err := repo.Update(ctx, registered); err != nil {
		t.Fatalf("simulate register update: %v", err)
	}

	// StatusFlusher's targeted heartbeat update, using the STALE
	// pre-registration copy for id/phase/heartbeat/version — must not
	// resurrect RegTokenHash or wipe AuthTokenHash/ClusterUID even though
	// the stale copy itself still carries the old (empty) values.
	now := time.Now()
	stale.Phase = "Online"
	stale.LastHeartbeatAt = &now
	stale.OperatorVersion = "v1.2.3"
	if err := repo.UpdateHeartbeat(ctx, stale.ID, stale.Phase, stale.LastHeartbeatAt, stale.OperatorVersion); err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}

	got, err := repo.GetByName(ctx, "prod")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.AuthTokenHash != "auth-hash" {
		t.Errorf("AuthTokenHash = %q, want auth-hash (must survive UpdateHeartbeat)", got.AuthTokenHash)
	}
	if got.RegTokenHash != "" {
		t.Errorf("RegTokenHash = %q, want empty (must not be resurrected)", got.RegTokenHash)
	}
	if got.ClusterUID != "uid-1" {
		t.Errorf("ClusterUID = %q, want uid-1", got.ClusterUID)
	}
	if got.Phase != "Online" || got.OperatorVersion != "v1.2.3" || got.LastHeartbeatAt == nil {
		t.Errorf("phase/version/heartbeat = %q/%q/%v, want Online/v1.2.3/non-nil", got.Phase, got.OperatorVersion, got.LastHeartbeatAt)
	}
}

// TestClusterRepoUpdateLabelsDoesNotClobberHeartbeat mirrors
// TestClusterRepoUpdateHeartbeatDoesNotClobberAuthFields above, but from the
// other direction: a console-triggered UpdateLabels must not stomp a
// concurrent StatusFlusher heartbeat write. Both methods only ever touch
// their own columns, so applying them in either order must leave both
// sets of fields intact.
func TestClusterRepoUpdateLabelsDoesNotClobberHeartbeat(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	ctx := context.Background()

	c := &dbrepo.Cluster{Name: "labeled", Phase: "Registering"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}

	now := time.Now()
	if err := repo.UpdateHeartbeat(ctx, c.ID, "Online", &now, "v1.2.3"); err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}
	if err := repo.UpdateLabels(ctx, c.ID, map[string]string{"env": "prod"}); err != nil {
		t.Fatalf("update labels: %v", err)
	}

	got, err := repo.GetByName(ctx, "labeled")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.Phase != "Online" || got.OperatorVersion != "v1.2.3" || got.LastHeartbeatAt == nil {
		t.Errorf("phase/version/heartbeat = %q/%q/%v, want Online/v1.2.3/non-nil (UpdateLabels must not clobber concurrent heartbeat write)", got.Phase, got.OperatorVersion, got.LastHeartbeatAt)
	}
	if got.Labels.Data()["env"] != "prod" {
		t.Errorf("labels[env] = %q, want prod", got.Labels.Data()["env"])
	}
}

func TestClusterRepoUpdateRegToken(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewClusterRepo(db)
	ctx := context.Background()

	c := &dbrepo.Cluster{Name: "reissue-me", Phase: "Registering", RegTokenHash: "old-hash"}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.UpdateRegToken(ctx, c.ID, "new-hash"); err != nil {
		t.Fatalf("update reg token: %v", err)
	}
	got, err := repo.GetByName(ctx, "reissue-me")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.RegTokenHash != "new-hash" {
		t.Errorf("RegTokenHash = %q, want new-hash", got.RegTokenHash)
	}
}
