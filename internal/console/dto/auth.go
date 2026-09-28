package dto

import dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"

type PasswordInput struct {
	Password string `json:"password"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SetupStatusView struct {
	Required bool `json:"required"`
}

type MeView struct {
	ID       int64       `json:"id"`
	Username string      `json:"username"`
	Role     dbrepo.Role `json:"role"`
	TenantID *int64      `json:"tenantId,omitempty"`
}
