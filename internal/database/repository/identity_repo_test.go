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

func TestIdentityUnactivatedAdminDoesNotAllowDisablingLastUsableAdmin(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewIdentityRepo(db)
	ctx := t.Context()
	admin, err := repo.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InitializeAdmin(ctx, "password-hash", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(ctx, &repository.User{Username: "pending-admin", Role: repository.RoleSystemAdmin, Status: repository.UserStatusActive}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetUserStatusAndRevoke(ctx, admin.ID, repository.UserStatusDisabled, time.Now()); !errors.Is(err, repository.ErrLastSystemAdmin) {
		t.Fatalf("SetUserStatusAndRevoke() error = %v, want %v", err, repository.ErrLastSystemAdmin)
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

func TestIdentityActivationAndSecuritySettings(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewIdentityRepo(db)
	ctx := t.Context()
	now := time.Now().UTC()
	tenant := &repository.Tenant{Name: "activation", Status: repository.UserStatusActive}
	if err := repo.CreateTenant(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	user := &repository.User{Username: "new-user", Role: repository.RoleViewer, Status: repository.UserStatusActive, TenantID: &tenant.ID}
	token := &repository.UserActivationToken{TokenHash: "activation-hash", ExpiresAt: now.Add(time.Hour)}
	if err := repo.CreateUserWithActivation(ctx, user, token); err != nil {
		t.Fatal(err)
	}
	if err := repo.ActivateUser(ctx, user.ID, token.TokenHash, "password-hash", now); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetUser(ctx, user.ID)
	if err != nil || got.PasswordHash != "password-hash" || got.PasswordChangedAt == nil {
		t.Fatalf("activated user = %#v, %v", got, err)
	}
	hashes, err := repo.ListPasswordHashes(ctx, user.ID, 5)
	if err != nil || len(hashes) != 1 || hashes[0] != "password-hash" {
		t.Fatalf("password history = %#v, %v", hashes, err)
	}
	settings, err := repo.GetSecuritySettings(ctx)
	if err != nil || settings.ActivationLifetimeHours != 24 || settings.AllowConcurrentSessions {
		t.Fatalf("security settings = %#v, %v", settings, err)
	}
}

func TestIdentityLoginSucceededReplacesExistingSession(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewIdentityRepo(db)
	ctx := t.Context()
	now := time.Now().UTC()
	admin, err := repo.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	first := &repository.Session{TokenHash: "first", ExpiresAt: now.Add(time.Hour), LastSeenAt: now}
	if err := repo.LoginSucceeded(ctx, admin.ID, first, now, "127.0.0.1", false); err != nil {
		t.Fatal(err)
	}
	second := &repository.Session{TokenHash: "second", ExpiresAt: now.Add(time.Hour), LastSeenAt: now}
	if err := repo.LoginSucceeded(ctx, admin.ID, second, now.Add(time.Second), "127.0.0.2", false); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := repo.GetActiveSession(ctx, "first", now); !errors.Is(err, repository.ErrInactiveSession) {
		t.Fatalf("first session error = %v", err)
	}
	if _, _, _, err := repo.GetActiveSession(ctx, "second", now); err != nil {
		t.Fatalf("second session error = %v", err)
	}
}

func TestIdentityConcurrentLoginLeavesOneActiveSession(t *testing.T) {
	db := newTestDB(t)
	repo := repository.NewIdentityRepo(db)
	ctx := t.Context()
	now := time.Now().UTC()
	admin, err := repo.GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, token := range []string{"concurrent-first", "concurrent-second"} {
		token := token
		go func() {
			<-start
			errs <- repo.LoginSucceeded(ctx, admin.ID, &repository.Session{TokenHash: token, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}, now, "127.0.0.1", false)
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	active := 0
	for _, token := range []string{"concurrent-first", "concurrent-second"} {
		if _, _, _, err := repo.GetActiveSession(ctx, token, now); err == nil {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("active concurrent sessions = %d, want 1", active)
	}
}
