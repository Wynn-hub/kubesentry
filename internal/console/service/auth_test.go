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
	settings              dbrepo.SecuritySettings
	activation            *dbrepo.UserActivationToken
	passwordHashes        []string
	lastLoginIP           string
	createdUser           *dbrepo.User
	createdActivation     *dbrepo.UserActivationToken
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
	f := &authRepoFake{
		admin:    dbrepo.User{Username: "admin", PasswordHash: hash, Role: dbrepo.RoleSystemAdmin, Status: dbrepo.UserStatusActive},
		sessions: map[string]*dbrepo.Session{},
		settings: dbrepo.SecuritySettings{ActivationLifetimeHours: 24, PasswordMinLength: 12, PasswordHistoryCount: 5, MaxFailedLoginAttempts: 5, LockoutDurationMinutes: 15, SessionIdleMinutes: 30, SessionAbsoluteMinutes: 480},
	}
	f.admin.ID = 1
	return f
}

func (f *authRepoFake) GetUserByUsername(_ context.Context, username string) (*dbrepo.User, error) {
	if username != f.admin.Username {
		return nil, errors.New("not found")
	}
	user := f.admin
	return &user, nil
}

func (f *authRepoFake) GetUser(_ context.Context, id int64) (*dbrepo.User, error) {
	if f.createdUser != nil && f.createdUser.ID == id {
		user := *f.createdUser
		return &user, nil
	}
	if f.admin.ID == id {
		user := f.admin
		return &user, nil
	}
	return nil, errors.New("not found")
}

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
	if f.tenant == nil {
		return &dbrepo.Tenant{Name: "tenant", Status: dbrepo.UserStatusActive}, nil
	}
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

func (f *authRepoFake) CreateUserWithActivation(_ context.Context, user *dbrepo.User, token *dbrepo.UserActivationToken) error {
	user.ID = 2
	token.UserID = user.ID
	userCopy, tokenCopy := *user, *token
	f.createdUser, f.createdActivation = &userCopy, &tokenCopy
	return nil
}

func (f *authRepoFake) ReplaceActivationToken(context.Context, int64, *dbrepo.UserActivationToken, time.Time) error {
	return nil
}

func (f *authRepoFake) GetActivationToken(_ context.Context, hash string) (*dbrepo.UserActivationToken, error) {
	if f.activation == nil || f.activation.TokenHash != hash || f.activation.UsedAt != nil {
		return nil, errors.New("not found")
	}
	token := *f.activation
	return &token, nil
}

func (f *authRepoFake) ActivateUser(_ context.Context, userID int64, tokenHash, passwordHash string, at time.Time) error {
	if f.activation == nil || f.activation.UserID != userID || f.activation.TokenHash != tokenHash || f.activation.UsedAt != nil || !f.activation.ExpiresAt.After(at) || f.admin.PasswordHash != "" {
		return errors.New("invalid activation")
	}
	f.admin.PasswordHash = passwordHash
	f.admin.PasswordChangedAt = &at
	f.admin.MustChangePassword = false
	f.activation.UsedAt = &at
	f.passwordHashes = append([]string{passwordHash}, f.passwordHashes...)
	return nil
}

func (f *authRepoFake) ListPasswordHashes(_ context.Context, _ int64, limit int) ([]string, error) {
	if limit > len(f.passwordHashes) {
		limit = len(f.passwordHashes)
	}
	return append([]string(nil), f.passwordHashes[:limit]...), nil
}

func (f *authRepoFake) RecordLoginFailure(_ context.Context, _ int64, maxAttempts, lockoutMinutes int, at time.Time) error {
	f.admin.FailedLoginCount++
	if f.admin.FailedLoginCount >= maxAttempts {
		locked := at.Add(time.Duration(lockoutMinutes) * time.Minute)
		f.admin.LockedUntil = &locked
	}
	return nil
}

func (f *authRepoFake) LoginSucceeded(_ context.Context, userID int64, session *dbrepo.Session, at time.Time, remoteIP string, allowConcurrent bool) error {
	if !allowConcurrent {
		_ = f.RevokeUserSessions(context.Background(), userID)
	}
	f.admin.LastLoginAt = &at
	f.admin.LastLoginIP = remoteIP
	f.admin.FailedLoginCount = 0
	f.admin.LockedUntil = nil
	f.lastLoginIP = remoteIP
	return f.CreateSession(context.Background(), session)
}

func (f *authRepoFake) ChangePassword(_ context.Context, _ int64, passwordHash string, keepSessionID *int64, at time.Time) error {
	f.admin.PasswordHash = passwordHash
	f.admin.PasswordChangedAt = &at
	f.admin.MustChangePassword = false
	f.passwordHashes = append([]string{passwordHash}, f.passwordHashes...)
	for _, session := range f.sessions {
		if keepSessionID == nil || session.ID != *keepSessionID {
			session.RevokedAt = &at
		}
	}
	return nil
}

func (f *authRepoFake) GetSecuritySettings(context.Context) (*dbrepo.SecuritySettings, error) {
	settings := f.settings
	return &settings, nil
}

func (f *authRepoFake) UpdateSecuritySettings(_ context.Context, settings *dbrepo.SecuritySettings) error {
	f.settings = *settings
	return nil
}

func (f *authRepoFake) UnlockUser(context.Context, int64, time.Time) error {
	f.admin.FailedLoginCount = 0
	f.admin.LockedUntil = nil
	return nil
}

func (f *authRepoFake) RequirePasswordChange(context.Context, int64, time.Time) error {
	f.admin.MustChangePassword = true
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

func TestAuthPasswordChangeKeepsCurrentSession(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewAuthService(repo)
	token, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := svc.Authenticate(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(t.Context(), principal, token, "another secure password"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(t.Context(), token); err != nil {
		t.Fatalf("current session error = %v", err)
	}
}

func TestAuthActivationHonorsExpiryAndConsumesCode(t *testing.T) {
	repo := newAuthRepoFake(t, "")
	now := time.Now().UTC()
	code := "one-time-activation-code"
	repo.activation = &dbrepo.UserActivationToken{UserID: repo.admin.ID, TokenHash: hashSessionToken(code), ExpiresAt: now.Add(time.Hour)}
	svc := NewAuthService(repo)
	svc.now = func() time.Time { return now }

	if err := svc.Activate(t.Context(), code, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Activate(t.Context(), code, "another secure password"); !errors.Is(err, ErrActivationInvalid) {
		t.Fatalf("reused activation error = %v", err)
	}

	repo.admin.PasswordHash = ""
	repo.activation = &dbrepo.UserActivationToken{UserID: repo.admin.ID, TokenHash: hashSessionToken("expired"), ExpiresAt: now.Add(-time.Minute)}
	if err := svc.Activate(t.Context(), "expired", "correct horse battery staple"); !errors.Is(err, ErrActivationInvalid) {
		t.Fatalf("expired activation error = %v", err)
	}
}

func TestAuthLockoutAndSuccessfulLoginMetadata(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	repo.settings.MaxFailedLoginAttempts = 2
	svc := NewAuthService(repo)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }

	for range 2 {
		_, _, _ = svc.Login(t.Context(), "admin", "wrong", "192.0.2.10")
	}
	if _, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple", "192.0.2.10"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("locked login error = %v", err)
	}
	now = now.Add(16 * time.Minute)
	if _, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple", "192.0.2.10"); err != nil {
		t.Fatal(err)
	}
	if repo.lastLoginIP != "192.0.2.10" || repo.admin.FailedLoginCount != 0 {
		t.Fatalf("login metadata = ip %q failures %d", repo.lastLoginIP, repo.admin.FailedLoginCount)
	}
}

func TestAuthSingleSessionAndConcurrentSessionSettings(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	svc := NewAuthService(repo)
	token1, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.Login(t.Context(), "admin", "correct horse battery staple", "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(t.Context(), token1); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("old session error = %v", err)
	}

	repo.settings.AllowConcurrentSessions = true
	token3, _, _ := svc.Login(t.Context(), "admin", "correct horse battery staple", "192.0.2.3")
	_, _, _ = svc.Login(t.Context(), "admin", "correct horse battery staple", "192.0.2.4")
	if _, err := svc.Authenticate(t.Context(), token3); err != nil {
		t.Fatalf("concurrent session rejected: %v", err)
	}
}

func TestPasswordPolicyHistoryAndExpiry(t *testing.T) {
	repo := newAuthRepoFake(t, "correct horse battery staple")
	repo.settings.PasswordLifetimeDays = 30
	repo.settings.PasswordHistoryCount = 5
	now := time.Now().UTC()
	changed := now.Add(-31 * 24 * time.Hour)
	repo.admin.PasswordChangedAt = &changed
	repo.passwordHashes = []string{repo.admin.PasswordHash}
	svc := NewAuthService(repo)
	svc.now = func() time.Time { return now }
	token, _, err := svc.Login(t.Context(), "admin", "correct horse battery staple", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := svc.Authenticate(t.Context(), token)
	if err != nil || !principal.MustChangePassword || principal.PasswordExpiresAt == nil {
		t.Fatalf("expired principal = %+v, %v", principal, err)
	}
	if err := svc.ChangePassword(t.Context(), principal, token, "correct horse battery staple"); !errors.Is(err, ErrPasswordReused) {
		t.Fatalf("reused password error = %v", err)
	}
	if err := svc.ChangePassword(t.Context(), principal, token, "another secure password"); err != nil {
		t.Fatal(err)
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
