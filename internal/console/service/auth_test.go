package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

type authRepoFake struct {
	mu                    sync.Mutex
	admin                 dbrepo.User
	tenant                *dbrepo.Tenant
	sessions              map[string]*dbrepo.Session
	tenantSessionsRevoked bool
}

func newAuthRepoFake(t *testing.T, password string) *authRepoFake {
	t.Helper()
	hash := ""
	if password != "" {
		value, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		hash = string(value)
	}
	return &authRepoFake{admin: dbrepo.User{Username: "admin", PasswordHash: hash, Role: dbrepo.RoleSystemAdmin, Status: dbrepo.UserStatusActive}, sessions: map[string]*dbrepo.Session{}}
}

func (f *authRepoFake) GetUserByUsername(_ context.Context, username string) (*dbrepo.User, error) {
	if username != f.admin.Username {
		return nil, errors.New("not found")
	}
	user := f.admin
	return &user, nil
}
func (f *authRepoFake) GetUser(context.Context, int64) (*dbrepo.User, error) { return &f.admin, nil }
func (f *authRepoFake) ListUsers(context.Context) ([]dbrepo.User, error) {
	return []dbrepo.User{f.admin}, nil
}

func (f *authRepoFake) ListUsersByTenant(context.Context, int64) ([]dbrepo.User, error) {
	return nil, nil
}
func (f *authRepoFake) ListTenants(context.Context) ([]dbrepo.Tenant, error) { return nil, nil }
func (f *authRepoFake) CreateTenant(context.Context, *dbrepo.Tenant) error   { return nil }

func (f *authRepoFake) SetTenantStatusAndRevoke(context.Context, int64, dbrepo.UserStatus, time.Time) error {
	f.tenantSessionsRevoked = true
	return nil
}
func (f *authRepoFake) CreateUser(context.Context, *dbrepo.User) error { return nil }
func (f *authRepoFake) SetUserStatusAndRevoke(context.Context, int64, dbrepo.UserStatus, time.Time) error {
	return nil
}

func (f *authRepoFake) SetUserPasswordAndRevoke(context.Context, int64, string, time.Time) error {
	return nil
}
func (f *authRepoFake) DeleteUserAndRevoke(context.Context, int64, time.Time) error { return nil }

func (f *authRepoFake) GetTenant(context.Context, int64) (*dbrepo.Tenant, error) {
	return f.tenant, nil
}

func (f *authRepoFake) UpdateUser(_ context.Context, user *dbrepo.User) error {
	f.admin = *user
	return nil
}

func (f *authRepoFake) InitializeAdmin(_ context.Context, hash string, _ time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.admin.PasswordHash != "" {
		return false, nil
	}
	f.admin.PasswordHash = hash
	return true, nil
}

func (f *authRepoFake) CreateSession(_ context.Context, session *dbrepo.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *session
	copy.ID = int64(len(f.sessions) + 1)
	f.sessions[session.TokenHash] = &copy
	return nil
}

func (f *authRepoFake) GetActiveSession(_ context.Context, hash string, now time.Time) (*dbrepo.Session, *dbrepo.User, *dbrepo.Tenant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	session := f.sessions[hash]
	if session == nil || session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return nil, nil, nil, dbrepo.ErrInactiveSession
	}
	copy := *session
	user := f.admin
	return &copy, &user, f.tenant, nil
}

func (f *authRepoFake) TouchSession(_ context.Context, id int64, at time.Time) error {
	for _, session := range f.sessions {
		if session.ID == id {
			session.LastSeenAt = at
		}
	}
	return nil
}

func (f *authRepoFake) RevokeSession(_ context.Context, id int64, at time.Time) error {
	for _, session := range f.sessions {
		if session.ID == id {
			session.RevokedAt = &at
		}
	}
	return nil
}

func (f *authRepoFake) RevokeUserSessions(_ context.Context, _ int64) error {
	now := time.Now()
	for _, session := range f.sessions {
		session.RevokedAt = &now
	}
	return nil
}

func TestInitializeAdminValidationAndRace(t *testing.T) {
	repo := newAuthRepoFake(t, "")
	svc := NewAuthService(repo)
	if err := svc.InitializeAdmin(t.Context(), "too-short"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("short password error = %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- svc.InitializeAdmin(t.Context(), "correct horse battery staple") }()
	}
	var success, conflict int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, ErrAlreadyInitialized):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestAuthLoginAndSessionLifecycle(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewAuthService(repo)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }

	if _, _, err := svc.Login(t.Context(), "admin", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("bad credential error = %v", err)
	}
	token, _, err := svc.Login(t.Context(), " ADMIN ", "correct horse battery staple")
	if err != nil || token == "" || repo.sessions[token] != nil || repo.sessions[hashSessionToken(token)] == nil {
		t.Fatalf("login token stored unsafely or failed: token=%q err=%v", token, err)
	}
	principal, err := svc.Authenticate(t.Context(), token)
	if err != nil || principal.Role != dbrepo.RoleSystemAdmin {
		t.Fatalf("Authenticate() = %+v, %v", principal, err)
	}
	now = now.Add(31 * time.Minute)
	if _, err := svc.Authenticate(t.Context(), token); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("idle session error = %v", err)
	}
}

func TestAuthRejectsDisabledUserAndTenant(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	repo.admin.Status = dbrepo.UserStatusDisabled
	svc := NewAuthService(repo)
	if _, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled user error = %v", err)
	}

	repo.admin.Status = dbrepo.UserStatusActive
	tenantID := int64(7)
	repo.admin.TenantID = &tenantID
	repo.tenant = &dbrepo.Tenant{Status: dbrepo.UserStatusDisabled}
	if _, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled tenant error = %v", err)
	}
}

func TestAuthPasswordChangeRevokesSessions(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewAuthService(repo)
	token, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(t.Context(), "admin", "another secure password"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(t.Context(), token); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("old session error = %v", err)
	}
}

func TestTenantStatusChangeRevokesSessions(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewIdentityService(repo)
	if err := svc.SetTenantStatus(t.Context(), dbrepo.Principal{Role: dbrepo.RoleSystemAdmin}, 7, dbrepo.UserStatusDisabled); err != nil {
		t.Fatal(err)
	}
	if !repo.tenantSessionsRevoked {
		t.Fatal("tenant sessions were not revoked")
	}
}
