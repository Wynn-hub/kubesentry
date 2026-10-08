package service

import (
	"context"

	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

type SecuritySettingsService struct {
	repo repository.IdentitySecurityRepository
}

func NewSecuritySettingsService(repo repository.IdentitySecurityRepository) *SecuritySettingsService {
	return &SecuritySettingsService{repo: repo}
}

func (s *SecuritySettingsService) Get(ctx context.Context, actor dbrepo.Principal) (*dto.SecuritySettingsView, error) {
	if actor.Role != dbrepo.RoleSystemAdmin {
		return nil, ErrForbidden
	}
	settings, err := s.repo.GetSecuritySettings(ctx)
	if err != nil {
		return nil, err
	}
	view := securitySettingsView(settings)
	return &view, nil
}

func (s *SecuritySettingsService) Update(ctx context.Context, actor dbrepo.Principal, input dto.SecuritySettingsInput) (*dto.SecuritySettingsView, error) {
	if actor.Role != dbrepo.RoleSystemAdmin {
		return nil, ErrForbidden
	}
	if !validSecuritySettings(input) {
		return nil, ErrInvalidIdentity
	}
	settings := &dbrepo.SecuritySettings{
		ActivationLifetimeHours: input.ActivationLifetimeHours,
		PasswordLifetimeDays:    input.PasswordLifetimeDays,
		PasswordMinLength:       input.PasswordMinLength,
		PasswordHistoryCount:    input.PasswordHistoryCount,
		MaxFailedLoginAttempts:  input.MaxFailedLoginAttempts,
		LockoutDurationMinutes:  input.LockoutDurationMinutes,
		SessionIdleMinutes:      input.SessionIdleMinutes,
		SessionAbsoluteMinutes:  input.SessionAbsoluteMinutes,
		AllowConcurrentSessions: input.AllowConcurrentSessions,
		UpdatedBy:               &actor.UserID,
	}
	if err := s.repo.UpdateSecuritySettings(ctx, settings); err != nil {
		return nil, err
	}
	view := securitySettingsView(settings)
	return &view, nil
}

func validSecuritySettings(v dto.SecuritySettingsInput) bool {
	return v.ActivationLifetimeHours >= 1 && v.ActivationLifetimeHours <= 720 &&
		v.PasswordLifetimeDays >= 0 && v.PasswordLifetimeDays <= 3650 &&
		v.PasswordMinLength >= 8 && v.PasswordMinLength <= 128 &&
		v.PasswordHistoryCount >= 0 && v.PasswordHistoryCount <= 24 &&
		v.MaxFailedLoginAttempts >= 1 && v.MaxFailedLoginAttempts <= 100 &&
		v.LockoutDurationMinutes >= 1 && v.LockoutDurationMinutes <= 10080 &&
		v.SessionIdleMinutes >= 1 && v.SessionIdleMinutes <= 10080 &&
		v.SessionAbsoluteMinutes >= 1 && v.SessionAbsoluteMinutes <= 43200 &&
		v.SessionIdleMinutes <= v.SessionAbsoluteMinutes
}

func securitySettingsView(v *dbrepo.SecuritySettings) dto.SecuritySettingsView {
	return dto.SecuritySettingsView{
		ActivationLifetimeHours: v.ActivationLifetimeHours,
		PasswordLifetimeDays:    v.PasswordLifetimeDays,
		PasswordMinLength:       v.PasswordMinLength,
		PasswordHistoryCount:    v.PasswordHistoryCount,
		MaxFailedLoginAttempts:  v.MaxFailedLoginAttempts,
		LockoutDurationMinutes:  v.LockoutDurationMinutes,
		SessionIdleMinutes:      v.SessionIdleMinutes,
		SessionAbsoluteMinutes:  v.SessionAbsoluteMinutes,
		AllowConcurrentSessions: v.AllowConcurrentSessions,
	}
}
