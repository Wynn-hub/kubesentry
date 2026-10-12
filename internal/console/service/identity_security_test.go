package service

import (
	"errors"
	"testing"
	"time"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestIdentityCreateUserReturnsOneTimeActivation(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewIdentityService(repo, nil)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	actor := dbrepo.Principal{UserID: 1, Role: dbrepo.RoleSystemAdmin}

	result, err := svc.CreateUser(t.Context(), actor, dto.CreateUserRequest{Username: " SECOND-ADMIN ", Role: dbrepo.RoleSystemAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if result.User.Username != "second-admin" || result.User.Activated || result.ActivationCode == "" {
		t.Fatalf("result = %+v", result)
	}
	if repo.createdUser == nil || repo.createdUser.PasswordHash != "" || repo.createdUser.TenantID != nil {
		t.Fatalf("created user = %+v", repo.createdUser)
	}
	if repo.createdActivation == nil || repo.createdActivation.TokenHash == result.ActivationCode || !repo.createdActivation.ExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("activation = %+v", repo.createdActivation)
	}
}

func TestIdentityCreateUserRoleAndTenantRules(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewIdentityService(repo, nil)
	tenantID := int64(9)
	tenantActor := dbrepo.Principal{Role: dbrepo.RoleTenantAdmin, TenantID: &tenantID}
	if _, err := svc.CreateUser(t.Context(), tenantActor, dto.CreateUserRequest{Username: "viewer", Role: dbrepo.RoleViewer, TenantID: &tenantID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateUser(t.Context(), tenantActor, dto.CreateUserRequest{Username: "admin", Role: dbrepo.RoleTenantAdmin, TenantID: &tenantID}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("tenant admin create error = %v", err)
	}
	if _, err := svc.CreateUser(t.Context(), dbrepo.Principal{Role: dbrepo.RoleSystemAdmin}, dto.CreateUserRequest{Username: "orphan", Role: dbrepo.RoleViewer}); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("orphan error = %v", err)
	}
}

func TestSecuritySettingsAuthorizationAndValidation(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewSecuritySettingsService(repo)
	admin := dbrepo.Principal{UserID: 1, Role: dbrepo.RoleSystemAdmin}
	viewer := dbrepo.Principal{UserID: 2, Role: dbrepo.RoleViewer}
	if _, err := svc.Get(t.Context(), viewer); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer get error = %v", err)
	}
	input := dto.SecuritySettingsInput{ActivationLifetimeHours: 24, PasswordLifetimeDays: 90, PasswordMinLength: 14, PasswordHistoryCount: 5, MaxFailedLoginAttempts: 5, LockoutDurationMinutes: 15, SessionIdleMinutes: 30, SessionAbsoluteMinutes: 480}
	updated, err := svc.Update(t.Context(), admin, input)
	if err != nil || updated.PasswordMinLength != 14 || repo.settings.UpdatedBy == nil || *repo.settings.UpdatedBy != admin.UserID {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	input.SessionIdleMinutes = 500
	if _, err := svc.Update(t.Context(), admin, input); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("invalid settings error = %v", err)
	}
}

func TestIdentityAdministrativeActions(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	locked := time.Now().UTC().Add(time.Hour)
	repo.admin.FailedLoginCount = 5
	repo.admin.LockedUntil = &locked
	svc := NewIdentityService(repo, nil)
	actor := dbrepo.Principal{UserID: 99, Role: dbrepo.RoleSystemAdmin}
	if err := svc.UnlockUser(t.Context(), actor, repo.admin.ID); err != nil {
		t.Fatal(err)
	}
	if repo.admin.FailedLoginCount != 0 || repo.admin.LockedUntil != nil {
		t.Fatalf("user remains locked: %+v", repo.admin)
	}
	if err := svc.ForcePasswordChange(t.Context(), actor, repo.admin.ID); err != nil {
		t.Fatal(err)
	}
	if !repo.admin.MustChangePassword {
		t.Fatal("password change was not required")
	}
}

func TestUserViewIncludesSecurityMetadata(t *testing.T) {
	now := time.Now().UTC()
	user := dbrepo.User{Username: "viewer", Role: dbrepo.RoleViewer, Status: dbrepo.UserStatusActive, PasswordHash: "hash", LastLoginAt: &now, LastLoginIP: "192.0.2.1", PasswordChangedAt: &now, MustChangePassword: true, FailedLoginCount: 2, LockedUntil: &now}
	user.ID = 7
	view := userView(user)
	if !view.Activated || view.LastLoginAt == nil || view.LastLoginIP != "192.0.2.1" || !view.MustChangePassword || view.FailedLoginCount != 2 || view.LockedUntil == nil {
		t.Fatalf("view = %+v", view)
	}
}
