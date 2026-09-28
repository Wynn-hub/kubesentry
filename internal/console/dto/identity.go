package dto

import dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"

type CreateTenantRequest struct {
	Name string `json:"name"`
}

type TenantStatusRequest struct {
	Status dbrepo.UserStatus `json:"status"`
}

type TenantView struct {
	ID     int64             `json:"id"`
	Name   string            `json:"name"`
	Status dbrepo.UserStatus `json:"status"`
}

type CreateUserRequest struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Role     dbrepo.Role `json:"role"`
	TenantID *int64      `json:"tenantId,omitempty"`
}

type UserStatusRequest struct {
	Status dbrepo.UserStatus `json:"status"`
}

type ResetPasswordRequest struct {
	Password string `json:"password"`
}

type UserView struct {
	ID       int64             `json:"id"`
	Username string            `json:"username"`
	Role     dbrepo.Role       `json:"role"`
	Status   dbrepo.UserStatus `json:"status"`
	TenantID *int64            `json:"tenantId,omitempty"`
}
