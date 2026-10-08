package service

import (
	"context"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func (s *AuthService) securitySettings(ctx context.Context) (*dbrepo.SecuritySettings, error) {
	return s.repo.GetSecuritySettings(ctx)
}
