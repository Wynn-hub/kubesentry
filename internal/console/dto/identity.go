package dto

import dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"

type CreateTenantRequest struct {
	Name string `json:"name"`
}

type TenantStatusRequest struct {
	Status dbrepo.UserStatus `json:"status" swaggertype:"string" enums:"active,disabled"`
}

type TenantView struct {
	ID     int64             `json:"id,string" swaggertype:"string"`
	Name   string            `json:"name"`
	Status dbrepo.UserStatus `json:"status" swaggertype:"string" enums:"active,disabled"`
}

type CreateUserRequest struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Role     dbrepo.Role `json:"role" swaggertype:"string" enums:"tenant_admin,viewer"`
	TenantID *int64      `json:"tenantId,omitempty,string" swaggertype:"string"`
}

type UserStatusRequest struct {
	Status dbrepo.UserStatus `json:"status" swaggertype:"string" enums:"active,disabled"`
}

type ResetPasswordRequest struct {
	Password string `json:"password"`
}

type UserView struct {
	ID       int64             `json:"id,string" swaggertype:"string"`
	Username string            `json:"username"`
	Role     dbrepo.Role       `json:"role" swaggertype:"string" enums:"system_admin,tenant_admin,viewer"`
	Status   dbrepo.UserStatus `json:"status" swaggertype:"string" enums:"active,disabled"`
	TenantID *int64            `json:"tenantId,omitempty,string" swaggertype:"string"`
}

type ClusterTenantRequest struct {
	TenantID *int64 `json:"tenantId,string" swaggertype:"string"`
}
