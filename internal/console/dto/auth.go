package dto

import "time"

import dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"

type PasswordInput struct {
	Password string `json:"password"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ActivateInput struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type SetupStatusView struct {
	Required bool `json:"required"`
}

type MeView struct {
	ID                 int64       `json:"id,string" swaggertype:"string"`
	Username           string      `json:"username"`
	Role               dbrepo.Role `json:"role" swaggertype:"string" enums:"system_admin,tenant_admin,viewer"`
	TenantID           *int64      `json:"tenantId,omitempty,string" swaggertype:"string"`
	LastLoginAt        *time.Time  `json:"lastLoginAt,omitempty"`
	PasswordExpiresAt  *time.Time  `json:"passwordExpiresAt,omitempty"`
	MustChangePassword bool        `json:"mustChangePassword"`
}
