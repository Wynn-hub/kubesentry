package service

import (
	"context"

	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
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
