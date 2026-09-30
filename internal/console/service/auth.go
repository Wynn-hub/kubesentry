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
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidPassword    = errors.New("password must be at least 12 characters")
	ErrAlreadyInitialized = errors.New("administrator password is already initialized")
	ErrAuthentication     = errors.New("authentication required")
)

type AuthService struct {
	repo             repository.IdentityRepository
	now              func() time.Time
	absoluteDuration time.Duration
	idleDuration     time.Duration
}

func NewAuthService(repo repository.IdentityRepository) *AuthService {
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

func (s *AuthService) SetupRequired(ctx context.Context) (bool, error) {
	admin, err := s.repo.GetUserByUsername(ctx, "admin")
	return err == nil && admin.PasswordHash == "", err
}

func (s *AuthService) InitializeAdmin(ctx context.Context, password string) error {
	if len(password) < MinimumPasswordLength {
		return ErrInvalidPassword
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

func (s *AuthService) Login(ctx context.Context, username, password string) (string, *dbrepo.User, error) {
	user, err := s.repo.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil || user.Status != dbrepo.UserStatusActive || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
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
	now := s.now()
	if err := s.repo.CreateSession(ctx, &dbrepo.Session{UserID: user.ID, TokenHash: tokenHash, ExpiresAt: now.Add(s.absoluteDuration), LastSeenAt: now}); err != nil {
		return "", nil, err
	}
	return token, user, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (dbrepo.Principal, error) {
	now := s.now()
	session, user, _, err := s.repo.GetActiveSession(ctx, hashSessionToken(token), now)
	if err != nil || now.Sub(session.LastSeenAt) > s.idleDuration {
		if err == nil {
			_ = s.repo.RevokeSession(ctx, session.ID, now)
		}
		return dbrepo.Principal{}, ErrAuthentication
	}
	if err := s.repo.TouchSession(ctx, session.ID, now); err != nil {
		return dbrepo.Principal{}, err
	}
	return dbrepo.Principal{UserID: user.ID, Username: user.Username, Role: user.Role, TenantID: user.TenantID}, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	session, _, _, err := s.repo.GetActiveSession(ctx, hashSessionToken(token), s.now())
	if err != nil {
		return nil
	}
	return s.repo.RevokeSession(ctx, session.ID, s.now())
}

func (s *AuthService) ChangePassword(ctx context.Context, username, password string) error {
	if len(password) < MinimumPasswordLength {
		return ErrInvalidPassword
	}
	user, err := s.repo.GetUserByUsername(ctx, username)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.PasswordHash = string(hash)
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	return s.repo.RevokeUserSessions(ctx, user.ID)
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
