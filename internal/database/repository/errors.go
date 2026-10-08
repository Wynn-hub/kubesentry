package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrConflict is returned by *UpdateCAS methods when the row's updated_at no
// longer matches the value the caller read — someone else wrote to the row
// in between. Callers map this to an HTTP 409.
var ErrConflict = errors.New("dbrepo: row was modified since it was read")

var (
	ErrLastSystemAdmin               = errors.New("at least one enabled system administrator is required")
	ErrClusterTenantInUse            = errors.New("cluster is targeted by an existing tenant distribution")
	ErrClusterNotFound               = errors.New("cluster not found")
	ErrClusterTenantTransferRequired = errors.New("cluster belongs to another tenant")
	ErrInactiveTenant                = errors.New("tenant is inactive")
)

// IsUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505) — the GORM-era equivalent of
// apierrors.IsAlreadyExists on a Create call against a uniqueIndex column
// (e.g. policies.name, policy_groups.name, policy_exceptions.name).
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
