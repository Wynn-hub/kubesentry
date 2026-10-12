package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

const MinimumPasswordLength = 12

var (
	ErrInvalidCredentials       = errors.New("invalid username or password")
	ErrInvalidPassword          = errors.New("password does not meet the minimum length requirement")
	ErrAlreadyInitialized       = errors.New("administrator password is already initialized")
	ErrAuthentication           = errors.New("authentication required")
	ErrActivationInvalid        = errors.New("activation code is invalid or expired")
	ErrAccountLocked            = errors.New("account is temporarily locked")
	ErrPasswordReused           = errors.New("password was used recently")
	ErrCurrentPasswordIncorrect = errors.New("current password is incorrect")
)

type AuthService struct {
	repo             repository.IdentitySecurityRepository
	now              func() time.Time
	absoluteDuration time.Duration
	idleDuration     time.Duration
}

func NewAuthService(repo repository.IdentitySecurityRepository) *AuthService {
	return &AuthService{repo: repo, now: time.Now, absoluteDuration: 24 * time.Hour, idleDuration: 30 * time.Minute}
}

func (s *AuthService) SetDurations(absolute, idle time.Duration) {
	if absolute > 0 {
		s.absoluteDuration = absolute
	}
	if idle > 0 {
		s.idleDuration = idle
	}
}

func (s *AuthService) SessionAbsoluteDuration(ctx context.Context) time.Duration {
	settings, err := s.securitySettings(ctx)
	if err == nil && settings.SessionAbsoluteMinutes > 0 {
		return time.Duration(settings.SessionAbsoluteMinutes) * time.Minute
	}
	return s.absoluteDuration
}

func (s *AuthService) SetupRequired(ctx context.Context) (bool, error) {
	admin, err := s.repo.GetUserByUsername(ctx, "admin")
	return err == nil && admin.PasswordHash == "", err
}

func (s *AuthService) InitializeAdmin(ctx context.Context, password string) error {
	settings, err := s.securitySettings(ctx)
	if err != nil {
		return err
	}
	if err := validatePasswordLength(password, settings); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	won, err := s.repo.InitializeAdmin(ctx, string(hash), s.now())
	if err != nil {
		return err
	}
	if !won {
		return ErrAlreadyInitialized
	}
	return nil
}

func (s *AuthService) Login(ctx context.Context, username, password string, remoteIPs ...string) (string, *dbrepo.User, error) {
	user, err := s.repo.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil || user.Status != dbrepo.UserStatusActive || user.PasswordHash == "" {
		return "", nil, ErrInvalidCredentials
	}
	now := s.now()
	if user.LockedUntil != nil && user.LockedUntil.After(now) {
		return "", nil, ErrAccountLocked
	}
	settings, err := s.securitySettings(ctx)
	if err != nil {
		return "", nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		_ = s.repo.RecordLoginFailure(ctx, user.ID, settings.MaxFailedLoginAttempts, settings.LockoutDurationMinutes, now)
		return "", nil, ErrInvalidCredentials
	}
	if user.TenantID != nil {
		tenant, tenantErr := s.repo.GetTenant(ctx, *user.TenantID)
		if tenantErr != nil || tenant.Status != dbrepo.UserStatusActive {
			return "", nil, ErrInvalidCredentials
		}
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", nil, err
	}
	absoluteDuration := time.Duration(settings.SessionAbsoluteMinutes) * time.Minute
	if absoluteDuration <= 0 {
		absoluteDuration = s.absoluteDuration
	}
	remoteIP := ""
	if len(remoteIPs) > 0 {
		remoteIP = remoteIPs[0]
	}
	if err := s.repo.LoginSucceeded(ctx, user.ID, &dbrepo.Session{UserID: user.ID, TokenHash: tokenHash, ExpiresAt: now.Add(absoluteDuration), LastSeenAt: now}, now, remoteIP, settings.AllowConcurrentSessions); err != nil {
		return "", nil, err
	}
	user.LastLoginAt = &now
	user.LastLoginIP = remoteIP
	user.FailedLoginCount = 0
	user.LockedUntil = nil
	return token, user, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (dbrepo.Principal, error) {
	now := s.now()
	session, user, _, err := s.repo.GetActiveSession(ctx, hashSessionToken(token), now)
	settings, settingsErr := s.securitySettings(ctx)
	if settingsErr != nil {
		return dbrepo.Principal{}, settingsErr
	}
	idleDuration := time.Duration(settings.SessionIdleMinutes) * time.Minute
	if idleDuration <= 0 {
		idleDuration = s.idleDuration
	}
	if err != nil || now.Sub(session.LastSeenAt) > idleDuration {
		if err == nil {
			_ = s.repo.RevokeSession(ctx, session.ID, now)
		}
		return dbrepo.Principal{}, ErrAuthentication
	}
	if err := s.repo.TouchSession(ctx, session.ID, now); err != nil {
		return dbrepo.Principal{}, err
	}
	expiresAt, forced := passwordExpiry(user, settings)
	if expiresAt != nil && !expiresAt.After(now) {
		forced = true
	}
	return dbrepo.Principal{UserID: user.ID, Username: user.Username, Role: user.Role, TenantID: user.TenantID, LastLoginAt: user.LastLoginAt, PasswordExpiresAt: expiresAt, MustChangePassword: forced}, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	session, _, _, err := s.repo.GetActiveSession(ctx, hashSessionToken(token), s.now())
	if err != nil {
		return nil
	}
	return s.repo.RevokeSession(ctx, session.ID, s.now())
}

func (s *AuthService) Activate(ctx context.Context, code, password string) error {
	settings, err := s.securitySettings(ctx)
	if err != nil {
		return err
	}
	if err := validatePasswordLength(password, settings); err != nil {
		return err
	}
	tokenHash := hashSessionToken(code)
	token, err := s.repo.GetActivationToken(ctx, tokenHash)
	if err != nil || !token.ExpiresAt.After(s.now()) {
		return ErrActivationInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.ActivateUser(ctx, token.UserID, tokenHash, string(hash), s.now()); err != nil {
		return ErrActivationInvalid
	}
	return nil
}

func (s *AuthService) ChangePassword(ctx context.Context, principal dbrepo.Principal, sessionToken, currentPassword, newPassword string) error {
	settings, err := s.securitySettings(ctx)
	if err != nil {
		return err
	}
	user, err := s.repo.GetUser(ctx, principal.UserID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
		return ErrCurrentPasswordIncorrect
	}
	if err := validatePasswordLength(newPassword, settings); err != nil {
		return err
	}
	hashes, err := s.repo.ListPasswordHashes(ctx, user.ID, settings.PasswordHistoryCount)
	if err != nil {
		return err
	}
	if passwordWasUsed(newPassword, append(hashes, user.PasswordHash)) {
		return ErrPasswordReused
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	var keepSessionID *int64
	if sessionToken != "" {
		session, _, _, sessionErr := s.repo.GetActiveSession(ctx, hashSessionToken(sessionToken), s.now())
		if sessionErr != nil || session.UserID != user.ID {
			return ErrAuthentication
		}
		keepSessionID = &session.ID
	}
	return s.repo.ChangePassword(ctx, user.ID, string(hash), keepSessionID, s.now())
}

func newSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashSessionToken(token), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
