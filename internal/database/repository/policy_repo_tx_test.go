package repository_test

import (
	"context"
	"errors"
	"testing"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func v1alpha1PolicyMatchFixture() v1alpha1.PolicyMatch {
	return v1alpha1.PolicyMatch{
		Operations: []string{"CREATE"},
		Resources: []v1alpha1.MatchResource{
			{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
		},
	}
}

// CreateWithVersion commits the policy row and its v1 snapshot atomically:
// both exist afterwards, and the snapshot's PolicyID is back-filled from the
// freshly assigned row ID.
func TestCreateWithVersion(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPolicyRepo(db)
	ctx := context.Background()

	p := &dbrepo.Policy{
		Name:            "tx-create",
		EnforcementMode: "audit",
		Rego:            "package kubesentry",
		Match:           datatypes.NewJSONType(v1alpha1PolicyMatchFixture()),
		CompileOK:       true,
	}
	v := &dbrepo.PolicyVersion{Version: 1, Rego: p.Rego}

	if err := repo.CreateWithVersion(ctx, p, v); err != nil {
		t.Fatalf("CreateWithVersion: %v", err)
	}
	if v.PolicyID != p.ID {
		t.Fatalf("snapshot PolicyID = %d, want back-filled row ID %d", v.PolicyID, p.ID)
	}
	got, err := repo.GetByName(ctx, "tx-create")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Name != "tx-create" {
		t.Fatalf("name = %q", got.Name)
	}
	versions, err := repo.ListVersions(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].Version != 1 {
		t.Fatalf("versions = %+v, want exactly v1", versions)
	}
}

// UpdateWithVersion bumps the row and appends the next snapshot atomically,
// and rejects a stale expectedUpdatedAt with ErrConflict.
func TestUpdateWithVersion(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPolicyRepo(db)
	ctx := context.Background()

	p := &dbrepo.Policy{
		Name:            "tx-update",
		EnforcementMode: "audit",
		Rego:            "package kubesentry",
		Match:           datatypes.NewJSONType(v1alpha1PolicyMatchFixture()),
		CompileOK:       true,
	}
	if err := repo.CreateWithVersion(ctx, p, &dbrepo.PolicyVersion{Version: 1, Rego: p.Rego}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fresh, err := repo.GetByName(ctx, "tx-update")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}

	fields := map[string]any{"description": "updated"}
	v2 := &dbrepo.PolicyVersion{Version: 2, Rego: fresh.Rego}
	if err := repo.UpdateWithVersion(ctx, fresh.ID, fresh.UpdatedAt, fields, v2); err != nil {
		t.Fatalf("UpdateWithVersion: %v", err)
	}
	if v2.PolicyID != fresh.ID {
		t.Fatalf("snapshot PolicyID = %d, want %d", v2.PolicyID, fresh.ID)
	}

	// A second writer holding the now-stale timestamp must be rejected.
	stale := &dbrepo.PolicyVersion{Version: 3, Rego: fresh.Rego}
	if err := repo.UpdateWithVersion(ctx, fresh.ID, fresh.UpdatedAt, fields, stale); !errors.Is(err, dbrepo.ErrConflict) {
		t.Fatalf("stale update err = %v, want ErrConflict", err)
	}
	versions, err := repo.ListVersions(ctx, fresh.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %d rows, want 2 — the rejected CAS must not have appended a snapshot", len(versions))
	}
}

// UpdateWithVersion must roll back the row update when the snapshot insert
// fails. This is the assertion that actually proves the transaction exists:
// the CAS succeeds first, so a non-transactional implementation would leave
// the row mutated while the snapshot insert was rejected.
func TestUpdateWithVersionRollsBackRowWhenSnapshotFails(t *testing.T) {
	db := newTestDB(t)
	repo := dbrepo.NewPolicyRepo(db)
	ctx := context.Background()

	p := &dbrepo.Policy{
		Name:            "tx-rollback",
		Description:     "original",
		EnforcementMode: "audit",
		Rego:            "package kubesentry",
		Match:           datatypes.NewJSONType(v1alpha1PolicyMatchFixture()),
		CompileOK:       true,
	}
	if err := repo.CreateWithVersion(ctx, p, &dbrepo.PolicyVersion{Version: 1, Rego: p.Rego}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fresh, err := repo.GetByName(ctx, "tx-rollback")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}

	// Version 1 already exists, so the unique index on (policy_id, version)
	// rejects this snapshot — after UpdateCAS has already succeeded.
	dup := &dbrepo.PolicyVersion{Version: 1, Rego: fresh.Rego}
	err = repo.UpdateWithVersion(ctx, fresh.ID, fresh.UpdatedAt,
		map[string]any{"description": "must not persist"}, dup)
	if err == nil {
		t.Fatal("expected the duplicate version insert to fail")
	}

	after, err := repo.GetByName(ctx, "tx-rollback")
	if err != nil {
		t.Fatalf("GetByName after rollback: %v", err)
	}
	if after.Description != "original" {
		t.Fatalf("description = %q, want %q — the row update was NOT rolled back, so the two writes are not atomic",
			after.Description, "original")
	}
	versions, err := repo.ListVersions(ctx, fresh.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("versions = %d rows, want 1", len(versions))
	}
}
