package service

import (
	"context"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

const (
	ownerSystem = "system"
	ownerTenant = "tenant"
)

func requestPrincipal(ctx context.Context) dbrepo.Principal {
	if principal, ok := dbrepo.PrincipalFromContext(ctx); ok {
		return principal
	}
	// Direct service callers are trusted internal code; HTTP always installs a principal.
	return dbrepo.Principal{Role: dbrepo.RoleSystemAdmin}
}

func requireSystemAdmin(ctx context.Context) error {
	if requestPrincipal(ctx).Role != dbrepo.RoleSystemAdmin {
		return ErrForbidden
	}
	return nil
}

func requireEditor(ctx context.Context) error {
	if requestPrincipal(ctx).Role == dbrepo.RoleViewer {
		return ErrForbidden
	}
	return nil
}

func authorizeOwner(ctx context.Context, ownerType string, tenantID *int64) error {
	principal := requestPrincipal(ctx)
	if principal.Role == dbrepo.RoleSystemAdmin {
		return nil
	}
	if principal.Role != dbrepo.RoleTenantAdmin || ownerType != ownerTenant || principal.TenantID == nil || tenantID == nil || *principal.TenantID != *tenantID {
		return ErrForbidden
	}
	return nil
}

func visibleTo(ctx context.Context, ownerType string, tenantID *int64) bool {
	principal := requestPrincipal(ctx)
	return principal.Role == dbrepo.RoleSystemAdmin || ownerType == ownerSystem || (principal.TenantID != nil && tenantID != nil && *principal.TenantID == *tenantID)
}
