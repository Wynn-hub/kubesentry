package service

import (
	"time"

	"golang.org/x/crypto/bcrypt"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func validatePasswordLength(password string, settings *dbrepo.SecuritySettings) error {
	minimum := MinimumPasswordLength
	if settings != nil && settings.PasswordMinLength > 0 {
		minimum = settings.PasswordMinLength
	}
	if len(password) < minimum {
		return ErrInvalidPassword
	}
	return nil
}

func passwordWasUsed(password string, hashes []string) bool {
	for _, hash := range hashes {
		if hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
			return true
		}
	}
	return false
}

func passwordExpiry(user *dbrepo.User, settings *dbrepo.SecuritySettings) (*time.Time, bool) {
	if settings == nil || settings.PasswordLifetimeDays <= 0 || user.PasswordChangedAt == nil {
		return nil, user.MustChangePassword
	}
	expiresAt := user.PasswordChangedAt.Add(time.Duration(settings.PasswordLifetimeDays) * 24 * time.Hour)
	return &expiresAt, user.MustChangePassword
}
