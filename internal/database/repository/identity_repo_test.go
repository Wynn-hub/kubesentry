package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestIdentityTenantAndUserCRUD(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewIdentityRepo(db)
	ctx := context.Background()

	tenant := &repository.Tenant{Name: "acme", Status: repository.UserStatusActive}
	if err := repo.CreateTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetTenant(ctx, tenant.ID); err != nil || got.Name != tenant.Name {
		t.Fatalf("GetTenant() = %+v, %v", got, err)
	}
	if err := repo.SetTenantStatusAndRevoke(ctx, tenant.ID, repository.UserStatusDisabled, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ListTenants(ctx); err != nil || len(got) != 1 || got[0].Status != repository.UserStatusDisabled {
		t.Fatalf("ListTenants() = %+v, %v", got, err)
	}

	user := &repository.User{Username: "alice", PasswordHash: "hash", Role: repository.RoleViewer, Status: repository.UserStatusActive, TenantID: &tenant.ID}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetUserByUsername(ctx, "alice"); err != nil || got.ID != user.ID {
		t.Fatalf("GetUserByUsername() = %+v, %v", got, err)
	}
	user.Status = repository.UserStatusDisabled
	if err := repo.UpdateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ListUsersByTenant(ctx, tenant.ID); err != nil || len(got) != 1 || got[0].Status != repository.UserStatusDisabled {
		t.Fatalf("ListUsersByTenant() = %+v, %v", got, err)
	}
}

func TestIdentityInitializeAdminIsAtomic(t *testing.T) {
	repo := repository.NewIdentityRepo(newTestDB(t))
	now := time.Now()
	won, err := repo.InitializeAdmin(context.Background(), "first", now)
	if err != nil || !won {
		t.Fatalf("first initialize = %v, %v", won, err)
	}
	won, err = repo.InitializeAdmin(context.Background(), "second", now)
	if err != nil || won {
		t.Fatalf("second initialize = %v, %v", won, err)
	}
}

func TestIdentitySessionLifecycle(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewIdentityRepo(db)
	ctx := context.Background()
	now := time.Now().UTC()
	admin, err := repo.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	session := &repository.Session{UserID: admin.ID, TokenHash: "token", ExpiresAt: now.Add(time.Hour), LastSeenAt: now}
	if err := repo.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	got, user, tenant, err := repo.GetActiveSession(ctx, "token", now)
	if err != nil || got.ID != session.ID || user.ID != admin.ID || tenant != nil {
		t.Fatalf("GetActiveSession() = %+v, %+v, %+v, %v", got, user, tenant, err)
	}
	if err := repo.TouchSession(ctx, session.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokeSession(ctx, session.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := repo.GetActiveSession(ctx, "token", now); !errors.Is(err, repository.ErrInactiveSession) {
		t.Fatalf("revoked session error = %v", err)
	}

	expired := &repository.Session{UserID: admin.ID, TokenHash: "expired", ExpiresAt: now.Add(-time.Second), LastSeenAt: now}
	if err := repo.CreateSession(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := repo.GetActiveSession(ctx, "expired", now); !errors.Is(err, repository.ErrInactiveSession) {
		t.Fatalf("expired session error = %v", err)
	}
}
