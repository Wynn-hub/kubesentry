package console

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505) — the GORM-era equivalent of
// apierrors.IsAlreadyExists on a Create call against a uniqueIndex column
// (e.g. policies.name, policy_groups.name, policy_exceptions.name).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
